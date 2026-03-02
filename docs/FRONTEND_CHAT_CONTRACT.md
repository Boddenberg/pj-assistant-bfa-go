# Frontend — Contrato do Chat v9.0.0

> Guia para integração do chat com onboarding no frontend React/Next.js.
>
> **Breaking changes vs v8:** `current_field` → `step` + `next_step`, history enriquecido com `step`/`validated`.

---

## Endpoint

```
POST /v1/chat              → cliente anônimo (onboarding)
POST /v1/chat/{customerId} → cliente autenticado
```

---

## 1. Request (front → BFA)

```json
{
  "query": "Quero abrir minha conta PJ",
  "history": [
    {
      "query": "Olá",
      "answer": "Olá! Como posso ajudar?",
      "step": null,
      "validated": null
    }
  ]
}
```

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `query` | `string` | ✅ | Mensagem do usuário |
| `history` | `HistoryEntry[]` | ❌ | Últimas mensagens da conversa (máx 5 são usadas) |

### HistoryEntry (v9 — enriquecido)

```typescript
interface HistoryEntry {
  query: string;
  answer: string;
  step: string | null;      // ✨ NOVO — qual campo do onboarding este turno representa
  validated: boolean | null; // ✨ NOVO — true = aceito, false = rejeitado, null = N/A
}
```

> **Nota:** O frontend DEVE incluir `step` e `validated` no histórico quando devolver.
> O BFA enriquece esses campos automaticamente na resposta — basta persistir e reenviar.

---

## 2. Response (BFA → front)

```typescript
interface ChatResponse {
  answer: string;                    // Mensagem para exibir ao usuário
  context: string | null;            // "onboarding" | "pix" | "general" | ...
  intent: string | null;             // "open_account" | "pix_transfer" | ...
  confidence: number;                // 0.0 a 1.0

  // ✨ v9.0.0
  step: string | null;               // Qual campo do onboarding está sendo tratado
  field_value: string | null;        // Valor extraído da resposta do usuário
  next_step: string | null;          // Próximo campo que será pedido

  account_data: AccountData | null;  // Dados da conta criada (só quando step="completed")
  suggested_actions: string[];       // Sugestões de ações
}

interface AccountData {
  customerId: string;   // UUID do cliente criado
  agencia: string;      // Ex: "0001"
  conta: string;        // Ex: "123456-7"
}
```

### Mudanças v8 → v9

| v8 | v9 | Descrição |
|----|-----|-----------|
| `current_field` | `step` | Renomeado |
| _(não existia)_ | `next_step` | Indica qual campo vem depois |
| `history[].query/answer` | `history[].query/answer/step/validated` | History enriquecido |

---

## 3. Valores de `step`

| Valor | Significado | O que o front faz |
|-------|-------------|-------------------|
| `null` | **Não é onboarding.** Conversa normal. | Exibir `answer` normalmente |
| `"cnpj"` | Pedindo/recebendo CNPJ | Exibir `answer` + progresso 10% |
| `"razaoSocial"` | Pedindo/recebendo Razão Social | Exibir `answer` + progresso 20% |
| `"nomeFantasia"` | Pedindo/recebendo Nome Fantasia | Exibir `answer` + progresso 30% |
| `"email"` | Pedindo/recebendo E-mail | Exibir `answer` + progresso 40% |
| `"representanteName"` | Pedindo/recebendo Nome do representante | Exibir `answer` + progresso 50% |
| `"representanteCpf"` | Pedindo/recebendo CPF | Exibir `answer` + progresso 60% |
| `"representantePhone"` | Pedindo/recebendo Telefone | Exibir `answer` + progresso 70% |
| `"representanteBirthDate"` | Pedindo/recebendo Data de nascimento | Exibir `answer` + progresso 80% |
| `"password"` | Pedindo/recebendo Senha | Exibir `answer` + progresso 90%. Input type=password |
| `"passwordConfirmation"` | Pedindo confirmação de senha | Exibir `answer` + progresso 95%. Input type=password |
| `"completed"` | **Conta criada!** 🎉 | Exibir `answer` + `account_data`. Redirecionar p/ login |
| `"error"` | Erro no cadastro (ex: CNPJ duplicado) | Exibir `answer` com mensagem de erro |

### Valores de `next_step`

| Valor | Significado |
|-------|-------------|
| `null` | Sem próximo campo (conversa normal, ou esperando resposta) |
| `"cnpj"` ... `"passwordConfirmation"` | Próximo campo a ser pedido |
| `"completed"` | Todos os campos coletados — conta será criada |

---

## 4. Lógica no Frontend (pseudocódigo)

```typescript
const response = await fetch('/v1/chat', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ query: userMessage, history }),
});

const data: ChatResponse = await response.json();

// Sempre exibir a mensagem
addMessage({ role: 'assistant', text: data.answer });

// ✨ v9: Atualizar histórico COM step + validated (o BFA já envia enriquecido)
// O front NÃO precisa calcular step/validated — basta persistir o que o BFA devolveu
// e reenviar no próximo request.
history.push({
  query: userMessage,
  answer: data.answer,
  step: data.step,         // pode ser null
  validated: null,         // o front não sabe se foi validado; o BFA controla isso
});

// Lógica condicional por step (era current_field)
if (data.step === null) {
  // Conversa normal, nada especial
}

else if (data.step === 'completed' && data.account_data) {
  // 🎉 Conta criada — mostrar tela de sucesso
  showAccountCreatedScreen({
    agencia: data.account_data.agencia,
    conta: data.account_data.conta,
    customerId: data.account_data.customerId,
  });
}

else if (data.step === 'error') {
  // Erro no cadastro — exibir alerta
  showErrorAlert(data.answer);
}

else if (data.step === 'password' || data.step === 'passwordConfirmation') {
  // Campo de senha — mudar input para type=password
  setInputType('password');
}

else {
  // Campo de onboarding em andamento — atualizar barra de progresso
  setInputType('text');
  updateProgress(data.step);
}
```

---

## 5. Barra de Progresso (opcional)

```typescript
const ONBOARDING_FIELDS = [
  'cnpj', 'razaoSocial', 'nomeFantasia', 'email',
  'representanteName', 'representanteCpf', 'representantePhone',
  'representanteBirthDate', 'password', 'passwordConfirmation',
];

function getProgress(step: string | null): number {
  if (!step) return 0;
  if (step === 'completed') return 100;
  const idx = ONBOARDING_FIELDS.indexOf(step);
  return idx >= 0 ? Math.round(((idx + 1) / ONBOARDING_FIELDS.length) * 100) : 0;
}

// Uso:
// getProgress('cnpj')                  → 10
// getProgress('email')                 → 40
// getProgress('passwordConfirmation')  → 100
// getProgress('completed')             → 100
// getProgress(null)                    → 0
```

---

## 6. Tela de Sucesso (quando `completed`)

Quando `step === "completed"`, o `account_data` contém:

```json
{
  "customerId": "a1b2c3d4-...",
  "agencia": "0001",
  "conta": "987654-3"
}
```

**Esse é o mesmo contrato do `POST /v1/auth/register`.** Se o front já tem uma tela de sucesso do registro manual, pode reutilizar o mesmo componente.

Exemplo:

```tsx
{data.step === 'completed' && data.account_data && (
  <div className="success-card">
    <h2>🎉 Conta criada com sucesso!</h2>
    <div className="account-info">
      <p><strong>Agência:</strong> {data.account_data.agencia}</p>
      <p><strong>Conta:</strong> {data.account_data.conta}</p>
    </div>
    <button onClick={() => router.push('/login')}>
      Fazer Login
    </button>
  </div>
)}
```

---

## 7. Resumo das Mudanças v8 → v9

| O que | v8 | v9 |
|-------|-----|-----|
| Campo de rastreio | `current_field` | `step` |
| Próximo campo | _(não existia)_ | `next_step` |
| History entries | `{query, answer}` | `{query, answer, step, validated}` |
| `account_data` | ✅ | ✅ (sem mudança) |
| Request body | `{query, history}` | `{query, history}` (sem mudança, mas history agora é enriquecido) |
| `answer` | Texto livre | **Sem mudança** — sempre contém a mensagem para o usuário |
| Registro | Automático pelo chat | **Sem mudança** |

---

## 8. Checklist do Front (v9)

- [ ] Renomear `current_field` → `step` em todos os tipos e lógica
- [ ] Adicionar `next_step` ao tipo `ChatResponse`
- [ ] Atualizar `HistoryEntry` com `step: string | null` e `validated: boolean | null`
- [ ] Persistir `step`/`validated` no history e reenviar no próximo request
- [ ] Quando `step === "completed"` + `account_data` → exibir tela de sucesso
- [ ] Quando `step === "password"` ou `"passwordConfirmation"` → input type=password
- [ ] Quando `step === "error"` → exibir alerta de erro
- [ ] (Opcional) Barra de progresso baseada no `step`
- [ ] (Opcional) Usar `next_step` para pré-carregar labels/ícones do próximo campo
