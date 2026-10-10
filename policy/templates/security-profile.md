# Perfil de segurança — <serviço>

- **Dono:** <time/pessoa responsável>
- **Tipo:** core | webapp | app externo | infraestrutura | biblioteca
- **Trust boundary:** <descrição curta>
- **Última revisão:** AAAA-MM-DD

## Dados

| Dado/coleção/arquivo | Classe | Fonte canônica | Finalidade | Retenção/exclusão |
|---|---|---|---|---|

## Autenticação e autorização

- Audiência recebida: `<audience>`
- Purposes aceitos: `<lista>`
- Scopes system aceitos: `<lista>`
- Decisões vivas consultadas: `<lista>`
- Comportamento de revogação: `<descrição>`

## Superfície

- Endpoints públicos: `<allowlist ou nenhum>`
- Endpoints administrativos: `<lista>`
- Webhooks/sockets/uploads: `<lista>`
- Peers chamados: `<serviço, finalidade, token>`
- Eventos publicados/consumidos: `<projeção e dados>`

## Armazenamento e ciclo de vida

- Banco/volumes/caches: `<lista>`
- Backup/exportação/esquecimento: `<regras>`
- Logs e auditoria: `<regras>`
- Local/CI/preview: `<fixtures e proibições>`

## Ameaças e controles

| Ameaça | Controle | Teste/gate |
|---|---|---|

## IA e processadores

- Integrações aprovadas: `<nenhuma ou registros>`
- Classes enviadas: `<lista>`
- Allowlist/limites/retenção: `<descrição>`

## Exceções vigentes

- `<nenhuma ou IDs + expiração>`

## Validação

```sh
<comandos>
```
