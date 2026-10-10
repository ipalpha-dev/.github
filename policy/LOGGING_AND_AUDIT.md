# Logs, auditoria e eventos

## Logs técnicos

Nunca registrar:

- Authorization/cookies/tokens/códigos/secrets;
- telefone, e-mail, documento, endereço, saúde, foto/embedding;
- request/response/body/header completo;
- prompt/input/output com dados pessoais;
- mensagens de erro de banco que possam carregar valores.

Registre código estável, serviço, operação, IDs internos mínimos, contagem, duração, resultado e correlation/event ID. Use redaction central antes de qualquer objeto potencialmente pessoal.

## Auditoria para o titular

Leituras e alterações de dados pessoais seguem o contrato do `persons-api`: ator, contexto, app/entry point, finalidade, operação e classes/campos — nunca valores. Logs do titular não são logs técnicos genéricos.

## Eventos e filas

- Evento contém projeção mínima e versionada.
- Invalidação leva ID/versão; cliente refaz leitura autorizada.
- Entrega é ao menos uma vez: handlers idempotentes.
- Dead letters podem conter dados; acesso restrito, inspeção controlada e retenção definida.
- Falha de publish não deve vazar payload no log.

## Retenção

Cada coleção de log/auditoria declara prazo, fundamento e exclusão. Métricas não usam labels de pessoa, app secret, template content ou mensagem. Exceção de log detalhado só pode existir em desenvolvimento explícito com dados sintéticos.

## Erros

Respostas usam status/reason/mensagem localizada e não ecoam parser, stack, fornecedor, query, path local ou fragmento da entrada. Stack completa fica apenas em canal técnico aprovado e redigido.
