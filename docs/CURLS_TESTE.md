# cURLs de Teste — BFA Go

> Troque `SEU_ID` pelo UUID do cliente e `CARD_ID` pelo UUID do cartão.

---

## Dev Tools (Massa de Teste)

### Adicionar saldo na conta
```bash
curl -X POST http://localhost:8080/v1/dev/add-balance \
  -H "Content-Type: application/json" \
  -d '{"customerId": "SEU_ID", "amount": 50000}'
```

### Definir limite de crédito pré-aprovado
```bash
curl -X POST http://localhost:8080/v1/dev/set-credit-limit \
  -H "Content-Type: application/json" \
  -d '{"customerId": "SEU_ID", "creditLimit": 100000}'
```

### Gerar transações aleatórias no extrato
```bash
curl -X POST http://localhost:8080/v1/dev/generate-transactions \
  -H "Content-Type: application/json" \
  -d '{"customerId": "SEU_ID", "count": 30, "period": "last-12-months"}'
```

### Criar compras no cartão de crédito
```bash
curl -X POST http://localhost:8080/v1/dev/add-card-purchase \
  -H "Content-Type: application/json" \
  -d '{"customerId": "SEU_ID", "cardId": "CARD_ID", "amount": 150.00, "mode": "random", "count": 10}'
```

Com mês específico:
```bash
curl -X POST http://localhost:8080/v1/dev/add-card-purchase \
  -H "Content-Type: application/json" \
  -d '{"customerId": "SEU_ID", "cardId": "CARD_ID", "amount": 200.00, "mode": "random", "count": 5, "targetMonth": "2026-02"}'
```

---

## Consultas

### Listar contas
```bash
curl http://localhost:8080/v1/customers/SEU_ID/accounts
```

### Extrato (transações)
```bash
curl "http://localhost:8080/v1/customers/SEU_ID/transactions"
curl "http://localhost:8080/v1/customers/SEU_ID/transactions?type=pix_sent&limit=10"
curl "http://localhost:8080/v1/customers/SEU_ID/transactions/summary"
```

### Listar cartões contratados
```bash
curl http://localhost:8080/v1/customers/SEU_ID/credit-cards
```

### Contratar cartão de crédito
```bash
curl -X POST http://localhost:8080/v1/cards/request \
  -H "Content-Type: application/json" \
  -d '{"customerId": "SEU_ID", "requestedLimit": 50000}'
```

### Fatura do cartão (mês atual)
```bash
curl http://localhost:8080/v1/customers/SEU_ID/credit-cards/CARD_ID/invoice
```

### Fatura do cartão (mês específico)
```bash
curl http://localhost:8080/v1/cards/CARD_ID/invoices/2026-03
```
