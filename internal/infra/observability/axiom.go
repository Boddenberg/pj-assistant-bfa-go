// Package observability — axiom.go implementa um Zap Core que envia
// logs em batch para o Axiom via HTTP POST.
//
// Funciona como um "tee" junto com o console core: o app continua logando
// no stdout (Railway captura) e ao mesmo tempo envia para o Axiom
// para busca, alertas e retenção de longo prazo.
//
// Configuração via env vars:
//
//	AXIOM_TOKEN   — API token do Axiom (xaat-...)
//	AXIOM_DATASET — nome do dataset (ex: pj-agent-logs)
//
// Se AXIOM_TOKEN não estiver definido, o core não é criado
// e nenhum log é enviado — zero impacto no app.
package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

// axiomCore é um zapcore.Core que bufferiza log entries e envia
// em batch via HTTP POST para o Axiom.
type axiomCore struct {
	// Configuração
	token    string
	endpoint string // https://api.axiom.co/v1/datasets/{dataset}/ingest
	client   *http.Client
	level    zapcore.Level

	// Buffer e sincronização (compartilhados entre clones via With())
	shared *axiomShared

	// campos fixos adicionados via With()
	fields []zapcore.Field

	// Batching
	maxBatchSize  int
	flushInterval time.Duration
}

// axiomShared contém estado mutável compartilhado entre o core
// original e seus clones criados por With().
type axiomShared struct {
	mu     sync.Mutex
	buffer []map[string]interface{}
	ticker *time.Ticker
	done   chan struct{}
}

// axiomConfig contém as opções de configuração do Axiom core.
type axiomConfig struct {
	Token         string
	Dataset       string
	Level         zapcore.Level
	MaxBatchSize  int
	FlushInterval time.Duration
}

// newAxiomCore cria um novo core que envia logs para o Axiom.
// Retorna nil se o token estiver vazio.
func newAxiomCore(cfg axiomConfig) *axiomCore {
	if cfg.Token == "" {
		return nil
	}

	if cfg.MaxBatchSize == 0 {
		cfg.MaxBatchSize = 100
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 2 * time.Second
	}

	endpoint := fmt.Sprintf("https://api.axiom.co/v1/datasets/%s/ingest", cfg.Dataset)

	core := &axiomCore{
		token:    cfg.Token,
		endpoint: endpoint,
		level:    cfg.Level,
		client:   &http.Client{Timeout: 10 * time.Second},
		shared: &axiomShared{
			buffer: make([]map[string]interface{}, 0, cfg.MaxBatchSize),
			ticker: time.NewTicker(cfg.FlushInterval),
			done:   make(chan struct{}),
		},
		maxBatchSize:  cfg.MaxBatchSize,
		flushInterval: cfg.FlushInterval,
	}

	// Goroutine de flush periódico
	go core.flushLoop()

	return core
}

// flushLoop faz flush periódico do buffer.
func (c *axiomCore) flushLoop() {
	for {
		select {
		case <-c.shared.ticker.C:
			c.flush()
		case <-c.shared.done:
			c.flush() // flush final antes de parar
			return
		}
	}
}

// Enabled implementa zapcore.Core — filtra por nível.
func (c *axiomCore) Enabled(level zapcore.Level) bool {
	return level >= c.level
}

// With implementa zapcore.Core — retorna um core com campos adicionais.
func (c *axiomCore) With(fields []zapcore.Field) zapcore.Core {
	return &axiomCore{
		token:         c.token,
		endpoint:      c.endpoint,
		client:        c.client,
		level:         c.level,
		shared:        c.shared, // ponteiro — compartilha buffer e mutex
		maxBatchSize:  c.maxBatchSize,
		flushInterval: c.flushInterval,
		fields:        append(c.fields[:len(c.fields):len(c.fields)], fields...),
	}
}

// Check implementa zapcore.Core.
func (c *axiomCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		ce = ce.AddCore(entry, c)
	}
	return ce
}

// Write implementa zapcore.Core — adiciona o log ao buffer.
func (c *axiomCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// Monta o log entry como mapa — Axiom aceita _time como timestamp ISO8601
	logEntry := map[string]interface{}{
		"_time":   entry.Time.UTC().Format(time.RFC3339Nano),
		"level":   entry.Level.String(),
		"message": entry.Message,
		"logger":  entry.LoggerName,
		"service": "bfa-go",
	}

	if entry.Caller.Defined {
		logEntry["caller"] = entry.Caller.TrimmedPath()
	}

	// Adiciona campos fixos (do With)
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range c.fields {
		f.AddTo(enc)
	}
	// Adiciona campos do log
	for _, f := range fields {
		f.AddTo(enc)
	}
	// Mescla os campos no entry
	for k, v := range enc.Fields {
		logEntry[k] = v
	}

	c.shared.mu.Lock()
	c.shared.buffer = append(c.shared.buffer, logEntry)
	bufLen := len(c.shared.buffer)
	shouldFlush := bufLen >= c.maxBatchSize
	c.shared.mu.Unlock()

	// Flush imediato em 3 cenários:
	// 1. Buffer atingiu maxBatchSize (batch cheio)
	// 2. Log de nível Error ou superior (queremos visibilidade imediata)
	// 3. Poucos itens acumulados (< 10) — não vale esperar
	if shouldFlush || entry.Level >= zapcore.ErrorLevel || bufLen <= 10 {
		go c.flush()
	}

	return nil
}

// Sync implementa zapcore.Core — faz flush do buffer.
func (c *axiomCore) Sync() error {
	c.flush()
	select {
	case <-c.shared.done:
		// já fechado
	default:
		close(c.shared.done)
	}
	c.shared.ticker.Stop()
	return nil
}

// flush envia os logs do buffer para o Axiom via HTTP POST.
func (c *axiomCore) flush() {
	c.shared.mu.Lock()
	if len(c.shared.buffer) == 0 {
		c.shared.mu.Unlock()
		return
	}
	// Copia e limpa o buffer
	batch := make([]map[string]interface{}, len(c.shared.buffer))
	copy(batch, c.shared.buffer)
	c.shared.buffer = c.shared.buffer[:0]
	c.shared.mu.Unlock()

	// Serializa como JSON array
	body, err := json.Marshal(batch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[axiom] json marshal error: %v (batch_size=%d)\n", err, len(batch))
		return
	}

	// POST para o Axiom
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[axiom] request creation error: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[axiom] flush error: %v (batch_size=%d)\n", err, len(batch))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "[axiom] flush rejected: status=%d (batch_size=%d)\n", resp.StatusCode, len(batch))
	}
}
