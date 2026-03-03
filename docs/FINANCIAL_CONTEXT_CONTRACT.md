# Financial Context — Contrato BFA ↔ Agent Python

> Documento de referência para a equipe do Agent Python.  
> Descreve os dados financeiros que o BFA envia junto com cada request de chat,  
> e como o Judge pode usar esses dados na avaliação posterior.

---

## 1. Visão Geral

O BFA agora enriquece cada `AgentRequest` com um campo `financial_context` contendo
dados reais do cliente. O agente pode usar esses dados para responder perguntas sobre
saldo, cartões, PIX, faturas, boletos e perfil da empresa — **sem precisar chamar APIs extras**.

### Fluxo

```
Frontend → BFA (busca contexto financeiro) → Agent Python (usa contexto para responder) → BFA (salva transcript + contexto)
```

- O contexto é buscado **antes** de chamar o agente, com timeout de 3s.
- Cada sub-contexto é independente: se um falhar, os demais são enviados normalmente.
- Para `customer_id = "anonymous"`, nenhum contexto é enviado (`financial_context` = `null`).
- O Judge roda **separadamente** — lê os transcripts salvos e avalia a qualidade.

---

## 2. Contrato: `AgentRequest`

```json
{
  "customer_id": "864bc2d5-02ce-4ca8-a6a9-e271024238b9",
  "query": "qual meu saldo?",
  "history": [],
  "validation_error": "",
  "collected_data": [],
  "financial_context": {
    "account": { ... },
    "cards": { ... },
    "pix": { ... },
    "billing": { ... },
    "profile": { ... },
    "fetched_at": "2026-03-04T10:30:00Z",
    "context_keys": ["account", "cards", "pix", "profile"]
  }
}
```

> **Nota**: `financial_context` pode ser `null` (anonymous) ou conter apenas alguns sub-contextos
> (ex.: se o cliente não tem cartões, `cards` será `null`).

---

## 3. Sub-Contextos

### 3.1 `account` — Conta Corrente

```json
{
  "account_id": "uuid",
  "branch": "0001",
  "account_number": "123456",
  "balance": 50000.00,
  "available_balance": 50000.00,
  "overdraft_limit": 0.00,
  "credit_limit": 100000.00,
  "available_credit_limit": 50000.00,
  "status": "active"
}
```

**Perguntas que o agente pode responder:**
- "Qual meu saldo?"
- "Quanto tenho disponível?"
- "Qual meu limite de crédito?"
- "Minha conta está ativa?"
- "Qual minha agência e conta?"

---

### 3.2 `cards` — Cartões de Crédito

```json
{
  "cards": [
    {
      "card_id": "uuid",
      "last4": "1234",
      "brand": "Visa",
      "card_type": "corporate",
      "status": "active",
      "credit_limit": 50000.00,
      "available_limit": 30000.00,
      "used_limit": 20000.00,
      "due_day": 15,
      "billing_day": 5
    }
  ],
  "invoices": [
    {
      "card_id": "uuid",
      "reference_month": "2026-03",
      "total_amount": 5000.00,
      "minimum_payment": 500.00,
      "due_date": "2026-03-15",
      "status": "open"
    }
  ]
}
```

**Perguntas que o agente pode responder:**
- "Quais meus cartões?"
- "Qual o limite do meu cartão?"
- "Quanto já usei no cartão?"
- "Qual a fatura aberta?"
- "Quando vence minha fatura?"
- "Qual o pagamento mínimo?"

**Notas:**
- `invoices` inclui apenas faturas com status `open`, `closed` ou `overdue`
- Faturas `paid` são omitidas para economizar payload

---

### 3.3 `pix` — Chaves e Transferências PIX

```json
{
  "keys": [
    {
      "key_type": "cnpj",
      "key_value": "12.345.678/0001-90",
      "status": "active"
    }
  ],
  "recent_transfers": [
    {
      "transfer_id": "uuid",
      "amount": 1500.00,
      "destination_name": "Fornecedor ABC",
      "status": "completed",
      "funded_by": "balance",
      "created_at": "2026-03-03T14:00:00Z"
    }
  ],
  "scheduled_transfers": [
    {
      "transfer_id": "uuid",
      "amount": 3000.00,
      "destination_name": "Aluguel Sala Comercial",
      "scheduled_for": "2026-03-10",
      "status": "pending"
    }
  ]
}
```

**Perguntas que o agente pode responder:**
- "Quais minhas chaves PIX?"
- "Fiz algum PIX recentemente?"
- "Tenho PIX agendado?"
- "Quanto transferi por PIX?"
- "Pra quem foi meu último PIX?"

---

### 3.4 `billing` — Boletos e Compras no Débito

```json
{
  "recent_bills": [
    {
      "bill_id": "uuid",
      "amount": 1200.00,
      "beneficiary": "CEMIG",
      "due_date": "2026-03-10",
      "status": "completed"
    }
  ],
  "recent_debits": [
    {
      "amount": 85.50,
      "merchant_name": "Papelaria Central",
      "category": "office_supplies",
      "date": "2026-03-02",
      "status": "completed"
    }
  ]
}
```

**Perguntas que o agente pode responder:**
- "Paguei algum boleto recentemente?"
- "Quais minhas compras no débito?"
- "Quanto gastei em material de escritório?"

---

### 3.5 `profile` — Perfil da Empresa

```json
{
  "customer_id": "uuid",
  "company_name": "Tech Solutions LTDA",
  "document": "12.345.678/0001-90",
  "segment": "small_business",
  "email": "contato@techsolutions.com.br"
}
```

**Perguntas que o agente pode responder:**
- "Qual o nome da minha empresa?"
- "Qual meu CNPJ?"
- "Qual meu segmento?"

---

## 4. Campo `context_keys`

O array `context_keys` lista quais sub-contextos foram preenchidos com sucesso.
Útil para o agente saber rapidamente quais dados estão disponíveis.

Valores possíveis: `"account"`, `"cards"`, `"pix"`, `"billing"`, `"profile"`

Exemplo: se o cliente não tem cartões e o billing falhou:
```json
"context_keys": ["account", "pix", "profile"]
```

---

## 5. Integração com o Judge (LLM-as-Judge)

O Judge **não** roda no fluxo do chat. Ele é executado de forma separada, lendo
os transcripts salvos no banco.

### Dados disponíveis para o Judge

Cada turno no `llm_transcripts` agora inclui:

| Coluna                      | Tipo     | Descrição                                          |
| --------------------------- | -------- | -------------------------------------------------- |
| `financial_context_keys`    | `text[]` | Quais sub-contextos foram enviados ao agente        |
| `financial_context_raw`     | `jsonb`  | JSON completo do FinancialContext enviado            |

### Como o Judge pode usar

No `EvaluateRequest`, cada `TranscriptEntry` agora inclui `financial_context_keys`:

```json
{
  "customer_id": "uuid",
  "conversation": [
    {
      "query": "qual meu saldo?",
      "answer": "Seu saldo atual é R$ 50.000,00...",
      "contexts": [],
      "financial_context_keys": ["account", "cards", "pix", "profile"],
      "latency_ms": 450,
      "created_at": "2026-03-04T10:30:01Z"
    }
  ]
}
```

**Critérios de avaliação sugeridos:**
- **Factual Accuracy**: A resposta bate com os dados do `financial_context_raw`?
- **Context Usage**: O agente usou os dados financeiros disponíveis?
- **Data Coverage**: O agente respondeu usando os contextos corretos para a pergunta?

---

## 6. Mapeamento Intent → Contextos Necessários

| Intent do usuário                      | Contextos usados               |
| -------------------------------------- | ------------------------------ |
| Saldo / disponível                     | `account`                      |
| Limite de crédito                      | `account`, `cards`             |
| Cartões / fatura / vencimento          | `cards`                        |
| PIX / chaves / transferências          | `pix`                          |
| Boletos / pagamentos                   | `billing`                      |
| Dados da empresa / CNPJ               | `profile`                      |
| Extrato / movimentação                 | `pix`, `billing`               |
| Visão geral financeira                 | `account`, `cards`, `pix`      |
| Agendamentos                          | `pix` (scheduled_transfers)    |
| Onboarding (anonymous)                | nenhum (financial_context=null) |

---

## 7. Exemplo Completo de Request

```json
POST /v1/chat
{
  "customer_id": "864bc2d5-02ce-4ca8-a6a9-e271024238b9",
  "query": "quanto tenho disponível no cartão e na conta?",
  "history": [
    {
      "query": "oi",
      "answer": "Olá! Como posso ajudar?"
    }
  ],
  "validation_error": "",
  "collected_data": [],
  "financial_context": {
    "account": {
      "account_id": "acc-001",
      "branch": "0001",
      "account_number": "123456",
      "balance": 50000.00,
      "available_balance": 50000.00,
      "overdraft_limit": 0.00,
      "credit_limit": 100000.00,
      "available_credit_limit": 50000.00,
      "status": "active"
    },
    "cards": {
      "cards": [
        {
          "card_id": "card-001",
          "last4": "4567",
          "brand": "Visa",
          "card_type": "corporate",
          "status": "active",
          "credit_limit": 50000.00,
          "available_limit": 30000.00,
          "used_limit": 20000.00,
          "due_day": 15,
          "billing_day": 5
        }
      ],
      "invoices": [
        {
          "card_id": "card-001",
          "reference_month": "2026-03",
          "total_amount": 20000.00,
          "minimum_payment": 2000.00,
          "due_date": "2026-03-15",
          "status": "open"
        }
      ]
    },
    "pix": null,
    "billing": null,
    "profile": {
      "customer_id": "864bc2d5-02ce-4ca8-a6a9-e271024238b9",
      "company_name": "Tech Solutions LTDA",
      "document": "12.345.678/0001-90",
      "segment": "small_business",
      "email": "contato@techsolutions.com.br"
    },
    "fetched_at": "2026-03-04T10:30:00Z",
    "context_keys": ["account", "cards", "profile"]
  }
}
```

### Resposta Esperada do Agente

```json
{
  "customer_id": "864bc2d5-02ce-4ca8-a6a9-e271024238b9",
  "answer": "Você tem **R$ 50.000,00** disponíveis na conta corrente e **R$ 30.000,00** de limite disponível no cartão Visa final 4567. Sua fatura atual de março está em R$ 20.000,00 com vencimento dia 15/03.",
  "context": "financial_query",
  "intent": "balance_and_cards",
  "confidence": 0.95,
  "step": null,
  "next_step": null,
  "rag_contexts": [],
  "suggested_actions": ["ver_fatura", "pagar_fatura"],
  "metadata": {},
  "timestamp": "2026-03-04T10:30:01Z"
}
```

---

## 8. Notas de Implementação

- **Timeout**: O BFA busca todos os contextos com timeout de 3 segundos. Se demorar mais, o contexto parcial é enviado.
- **Null safety**: Cada sub-contexto pode ser `null`. O agente deve sempre verificar antes de usar.
- **Dados recentes**: PIX, boletos e débitos retornam os 10 itens mais recentes (page 1, pageSize 10).
- **Faturas**: Apenas faturas com status `open`, `closed` ou `overdue` são incluídas.
- **Persistência**: O contexto é salvo na tabela `llm_transcripts` como `financial_context_keys` (text[]) e `financial_context_raw` (jsonb).
