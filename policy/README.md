# Política canônica de desenvolvimento seguro do IPAlpha

- **Versão:** 1.0.0
- **Estado:** proposta local para revisão
- **Dono:** segurança/plataforma IPAlpha

Este diretório é a fonte canônica para humanos e agentes de IA. O setup o copia para `.ipalpha/policy/` e cria `AGENTS.md` na raiz do workspace, para que as regras estejam disponíveis mesmo sem rede.

## Precedência

1. `SECURITY_BASELINE.md` e esta política canônica;
2. contratos do serviço dono do recurso;
3. perfil de segurança do repositório;
4. requisitos funcionais da tarefa;
5. preferências e exemplos.

Uma regra inferior não pode enfraquecer uma superior. README, comentário e código legado não criam exceção. Divergências devem falhar para o lado mais restritivo e ser registradas para correção.

## Linguagem normativa

- **DEVE / MUST:** obrigatório; o gate deve bloquear quando tecnicamente verificável.
- **NÃO DEVE / MUST NOT:** proibido.
- **DEVERIA / SHOULD:** padrão esperado; desvio precisa de justificativa.
- **PODE / MAY:** opção segura dentro dos limites anteriores.

## Ordem de leitura por tarefa

Sempre:

1. `SECURITY_BASELINE.md`
2. `DATA_CLASSIFICATION.md`
3. `SECURE_DELIVERY.md`

Depois, conforme o trabalho:

| Tarefa | Leia também |
|---|---|
| login, token, sessão, papel, scope | `IDENTITY_AND_AUTHORIZATION.md` |
| app, projeto, consentimento, aprovação | `APP_TRUST_AND_APPROVAL.md` |
| IA, importação inteligente, fornecedor | `AI_AND_EXTERNAL_PROCESSORS.md` |
| configuração ou credencial | `SECRETS_AND_CONFIGURATION.md` |
| logs, eventos, auditoria | `LOGGING_AND_AUDIT.md` |
| frontend, PWA, mobile, offline | `FRONTEND_MOBILE_AND_OFFLINE.md` |
| API, upload, webhook, socket | `API_AND_INTEGRATION_SECURITY.md` |
| banco, arquivo, backup, exclusão | `DATA_LIFECYCLE.md` |
| local, teste, CI ou preview | `DEVELOPMENT_ENVIRONMENTS.md` |
| suspeita de incidente | `INCIDENT_RESPONSE.md` |
| desvio necessário | `SECURITY_EXCEPTIONS.md` |

## Donos dos contratos

- `auth-api`: identidade, apps, entry points, consentimento, credenciais, sessões, emissão e revogação.
- `persons-api`: pessoas, contatos, documentos, saúde, família, arquivos pessoais e logs LGPD.
- `projects-api`: projetos, edições, papéis, memberships, políticas de acesso e templates de projeto/app.
- `organizations-api`: organograma e assignments; somente decisões explicitamente sancionadas podem derivar autorização.
- `places-api`: locais, recursos, reservas e políticas de uso dentro do contexto organizacional/projeto.
- `notifications-api`: resolução de contatos e entrega de SMS/e-mail.
- `dispatch-api`: invalidações e canais ao vivo; não decide o conteúdo visível.
- `developers-api`: solicitações, revisão e workflow durável de provisionamento; não guarda credenciais.
- `ai-api`: gateway institucional de IA, ledger e limites; apps externos não herdam acesso.

## Perfis e exceções

Cada repositório deve adotar `templates/repository-AGENTS.md` e `templates/security-profile.md`. Exceções seguem `SECURITY_EXCEPTIONS.md`, têm dono e expiração e nunca podem existir apenas em comentário ou chat.

## Arquivos legíveis por máquina

`policy.yaml` registra a versão, classes, invariantes e gates mínimos. Ele não substitui estes documentos; serve para automação e detecção de drift.
