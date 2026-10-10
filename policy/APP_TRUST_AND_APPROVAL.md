# Confiança, solicitações e aprovação de aplicativos

## Separações obrigatórias

- App não é projeto; um pode existir sem o outro.
- Aprovação de diretório/banner não concede dados, token, vínculo ou endosso institucional.
- App consent e project membership são independentes.
- Desenvolvedor nunca cria `church` ou `prefill`; o portal provisiona entry points externos.
- Somente superusuário designa entry point institucional, com auditoria.

## Fluxo canônico

1. Applicant elegível é verificado ao vivo no `persons-api`; elegibilidade não é claim persistido.
2. Developers recebe request tipado e idempotente.
3. Reviewer steward/superuser usa token `church_access`, full e revogável.
4. Aprovação pode reduzir, nunca ampliar além do pedido.
5. Workflow durável provisiona no dono (`auth-api`, `projects-api`) com CAS, versão e chaves de idempotência.
6. Recursos pendentes ficam invisíveis até ativação.
7. Credencial confidencial é emitida pelo `auth-api` ao owner, uma vez; Developers não a armazena.

## Tipos de aprovação

| Decisão | Aprovador mínimo |
|---|---|
| finalidade, linguagem e uso ministerial | responsável pastoral/produto |
| campos, kinds, audiência, scope e retenção | segurança + responsável pelos dados |
| implementação de auth/criptografia | revisor técnico de segurança |
| biometria ou processador de IA | segurança + responsável institucional/LGPD |
| church entry point/system client | superusuário + revisão técnica |
| deploy de produção | operação/plataforma autorizada |

A aprovação pastoral não substitui revisão técnica; revisão técnica não decide finalidade pastoral.

## Mudanças

- Widening de dados/audiências exige review e novo consentimento.
- Narrowing deve surtir efeito imediatamente e revogar o que ficou amplo.
- Versão base divergente impede last-write-wins.
- Suspensão/revogação corta tokens e canais; resume não revive token antigo.
- Exceções e conflitos de interesse são registrados.

## Evidência

Guardar request, revisão, versão, ator, mensagem ao applicant, nota interna separada, operação e resultado. Projeções públicas nunca incluem pessoas, notas internas, secrets, grants ou configuração privada.
