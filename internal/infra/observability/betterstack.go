// Package observability — betterstack.go implementa um Zap Core que envia
// logs em batch para o Better Stack via HTTP POST.
//
// Funciona como um "tee" junto com o console core: o app continua logando
// no stdout (Railway captura) e ao mesmo tempo envia para o Better Stack
// para busca, alertas e retenção de longo prazo.
//
// Configuração via env vars:
//
//	BETTERSTACK_SOURCE_TOKEN  — token do Source HTTP criado no Better Stack
//	BETTERSTACK_INGEST_URL    — URL de ingestão (ex: https://s1234567.eu-fsn-3.betterstackdata.com)
//
// Se BETTERSTACK_SOURCE_TOKEN não estiver definido, o core não é criado
// e nenhum log é enviado — zero impacto no app.
package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

// betterstackCore é um zapcore.Core que bufferiza log entries e envia
// em batch via HTTP POST para o Better Stack.
type betterstackCore struct {
	// Configuração
	token    string
	endpoint string
	client   *http.Client
	level    zapcore.Level

	// Buffer e sincronização
	mu      sync.Mutex
	buffer  []map[string]interface{}
	ticker  *time.Ticker
	done    chan struct{}
	fields  []zapcore.Field // campos fixos adicionados via With()

	// Batching
	maxBatchSize int
	flushInterval time.Duration
}

// betterstackConfig contém as opções de configuração do Better Stack core.
type betterstackConfig struct {
	Token         string
	Endpoint      string
	Level         zapcore.Level
	MaxBatchSize  int
	FlushInterval time.Duration
}

// newBetterstackCore cria um novo core que envia logs para o Better Stack.
// Retorna nil se o token estiver vazio.
func newBetterstackCore(cfg betterstackConfig) *betterstackCore {
	if cfg.Token == "" {
		return nil
	}

	if cfg.MaxBatchSize == 0 {
		cfg.MaxBatchSize = 100
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	core := &betterstackCore{
		token:         cfg.Token,
		endpoint:      cfg.Endpoint,
		level:         cfg.Level,
		client:        &http.Client{Timeout: 10 * time.Second},
		buffer:        make([]map[string]interface{}, 0, cfg.MaxBatchSize),
		ticker:        time.NewTicker(cfg.FlushInterval),
		done:          make(chan struct{}),
		maxBatchSize:  cfg.MaxBatchSize,
		flushInterval: cfg.FlushInterval,
	}

	// Goroutine de flush periódico
	go core.flushLoop()

	return core
}

// flushLoop faz flush periódico do buffer.
func (c *betterstackCore) flushLoop() {
	for {
		select {
		case <-c.ticker.C:
			c.flush()
		case <-c.done:
			c.flush() // flush final antes de parar
			return
		}
	}
}

// Enabled implementa zapcore.Core — filtra por nível.
func (c *betterstackCore) Enabled(level zapcore.Level) bool {
	return level >= c.level
}

// With implementa zapcore.Core — retorna um core com campos adicionais.
func (c *betterstackCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &betterstackCore{
		token:         c.token,
		endpoint:      c.endpoint,
		client:        c.client,
		level:         c.level,
		buffer:        c.buffer,
		ticker:        c.ticker,
		done:          c.done,
		maxBatchSize:  c.maxBatchSize,
		flushInterval: c.flushInterval,
		fields:        append(c.fields[:len(c.fields):len(c.fields)], fields...),
	}
	// Compartilha o mesmo mutex para o buffer
	clone.mu = c.mu
	return clone
}

// Check implementa zapcore.Core.
func (c *betterstackCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		ce = ce.AddCore(entry, c)
	}
	return ce
}

// Write implementa zapcore.Core — adiciona o log ao buffer.
func (c *betterstackCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// Monta o log entry como mapa
	logEntry := map[string]interface{}{
		"dt":      entry.Time.UTC().Format("2006-01-02 15:04:05 UTC"),
		"level":   entry.Level.String(),
		"message": entry.Message,
		"logger":  entry.LoggerName,
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

	c.mu.Lock()
	c.buffer = append(c.buffer, logEntry)
	shouldFlush := len(c.buffer) >= c.maxBatchSize
	c.mu.Unlock()

	if shouldFlush {
		go c.flush()
	}

	return nil
}

// Sync implementa zapcore.Core — faz flush do buffer.
func (c *betterstackCore) Sync() error {
	c.flush()
	close(c.done)
	c.ticker.Stop()
	return nil
}

// flush envia os logs do buffer para o Better Stack via HTTP POST.
func (c *betterstackCore) flush() {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	// Copia e limpa o buffer
	batch := make([]map[string]interface{}, len(c.buffer))
	copy(batch, c.buffer)
	c.buffer = c.buffer[:0]
	c.mu.Unlock()

	// Serializa como JSON array
	body, err := json.Marshal(batch)
	if err != nil {
		// Silenciosamente descarta — não podemos logar aqui (loop infinito)
		return
	}

	// POST para o Better Stack
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}
