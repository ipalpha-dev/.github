# APIs e integrações

## HTTP

- DTO/schema allowlist; campos desconhecidos são rejeitados em operações sensíveis.
- Limite de body antes do parse; JSON inválido e encoding não suportado recebem reason estável.
- IDs, paginação, cursor e filtros têm limite e validação.
- CORS usa origens exatas; prefira same-origin.
- Endpoints públicos constam no perfil e em allowlist legível por máquina.
- Respostas com token, code, proof, contato ou dado pessoal usam `Cache-Control: no-store`.

## Uploads e conteúdo

- Limite de bytes no servidor.
- Tipo detectado por magic bytes quando segurança depende dele; não confiar no MIME/nome.
- Nome de arquivo nunca vira path.
- HTML é sanitizado no servidor por allowlist; sem scripts, handlers, `data:` ou CSS livre.
- Imagem processada tem limites de dimensão/memória e decoder isolado.

## Webhooks

- HTTPS em produção, URL canônica, sem redirect.
- Proteção SSRF no cadastro e em cada resolução/conexão, com DNS pinado quando aplicável.
- HMAC sobre bytes crus, comparação em tempo constante, delivery ID e idempotência.
- Responder rápido; processamento idempotente/reconciliável.
- Retry somente para falha transitória e com o mesmo ID.

## Eventos e WebSocket

- Eventos de browser são invalidações; dados vêm do dono com token próprio.
- Token do socket e owner token devem representar o mesmo principal/contexto.
- Revalidar revogação antes de push sensível e periodicamente.
- App channel é servidor-servidor, um por app, com system client app-bound.
- Canais não são duráveis; reconexão faz snapshot/reconciliação.

## Chamadas a peers

- Timeout e limites explícitos; não incluir peer em readiness.
- 401 não é retry; 403 não é contornado; 5xx usa backoff somente se idempotente.
- Circuit breaker nunca converte falha em autorização.
- Não propagar mensagem interna do peer ao usuário.
