# Abertura de Conta PJ — Dados Necessários

## O que é

Fluxo conversacional para abrir uma conta PJ via chat.
O usuário conversa com a IA e fornece os dados em 3 etapas.

Ao final, o sistema cria: **cliente + conta corrente + credenciais de acesso**.

---

## Dados obrigatórios (9 campos, 3 etapas)

### Etapa 1 — Empresa

| Campo | JSON | Formato | Exemplo |
|-------|------|---------|---------|
| CNPJ | `cnpj` | 14 dígitos (sem máscara) | `"12345678000190"` |
| Razão Social | `razaoSocial` | texto livre | `"Tech Solutions LTDA"` |
| Nome Fantasia | `nomeFantasia` | texto livre | `"Tech Solutions"` |
| E-mail | `email` | email válido | `"contato@techsolutions.com.br"` |

### Etapa 2 — Responsável

| Campo | JSON | Formato | Exemplo |
|-------|------|---------|---------|
| Nome completo | `representanteName` | texto livre | `"Maria Oliveira"` |
| CPF | `representanteCpf` | 11 dígitos (sem máscara) | `"98765432100"` |
| Telefone | `representantePhone` | com DDD | `"+55 11 99999-0000"` |
| Data de nascimento | `representanteBirthDate` | YYYY-MM-DD | `"1985-03-20"` |

### Etapa 3 — Senha

| Campo | JSON | Formato | Exemplo |
|-------|------|---------|---------|
| Senha | `password` | exatamente 6 dígitos numéricos | `"142536"` |

---

## Regras de validação

- **CNPJ**: deve ser único (se já existe, retorna erro "CNPJ já cadastrado")
- **Senha**: exatamente 6 caracteres, todos numéricos (0-9)
- **CNPJ e CPF**: o sistema remove pontuação automaticamente — aceita com ou sem máscara

---

## O que é criado ao final

Quando os 9 campos são coletados com sucesso, o sistema cria automaticamente:

1. **Cliente** (tabela `customers`) — com CNPJ, razão social, nome fantasia, email, dados do responsável
2. **Conta corrente** (tabela `accounts`) — agência e número gerados automaticamente
3. **Credenciais** (tabela `credentials`) — CPF do responsável como login, senha com hash bcrypt

## Resposta de sucesso

```json
{
  "customerId": "uuid-gerado",
  "agencia": "0001",
  "conta": "123456-7",
  "message": "Conta criada com sucesso"
}
```

---

## Rotas disponíveis

### Via chat (conversacional)

```bash
curl -X POST \
  https://pj-assistant-bfa-go-production.up.railway.app/v1/chat/{customerId} \
  -H "Content-Type: application/json" \
  -d '{"query": "Quero abrir uma conta PJ"}'
```

A IA conduz a conversa pedindo os dados etapa por etapa.

### Via API direta (todos os dados de uma vez)

```bash
curl -X POST \
  https://pj-assistant-bfa-go-production.up.railway.app/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "cnpj": "12345678000190",
    "razaoSocial": "Tech Solutions LTDA",
    "nomeFantasia": "Tech Solutions",
    "email": "contato@techsolutions.com.br",
    "representanteName": "Maria Oliveira",
    "representanteCpf": "98765432100",
    "representantePhone": "+55 11 99999-0000",
    "representanteBirthDate": "1985-03-20",
    "password": "142536"
  }'
```

---

## Erros possíveis

| Status | Motivo |
|--------|--------|
| 400 | Campo obrigatório faltando ou formato inválido |
| 400 | Senha não tem 6 dígitos numéricos |
| 409 | CNPJ já cadastrado |
| 502 | Agent Python fora do ar (rota de chat) |

---

## Resumo para a IA

> Você precisa coletar **9 dados** para abrir a conta:
> CNPJ, razão social, nome fantasia, e-mail, nome do responsável, CPF, telefone, data de nascimento e senha de 6 dígitos.
> Peça os dados em ordem (empresa → responsável → senha). Não peça nada além disso.
