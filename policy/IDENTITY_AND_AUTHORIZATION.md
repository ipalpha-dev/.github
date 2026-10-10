# Identidade e autorização

## Modelo

- `auth-api` prova identidade, registra consentimento, emite tokens e revoga.
- Cada API verifica localmente assinatura ES256, issuer exato, audiência única, purpose, claims, tempo e revogação.
- Não há introspecção por request nem caminho “decode only”.
- Rotas declaram purposes aceitos e aplicam seus guards adicionais.

## Propósitos

- `church_access`: contexto institucional de entry point autorizado; papel real e `access: full|read`.
- `external_access`: app externo; `role=person` sempre; dados próprios limitados ao consentido e outros dados limitados pela política viva do projeto.
- `system_access`: serviço a serviço; scope exato e, quando app-bound, `appId` obrigatório e restritivo.
- Tokens delegados/one-time possuem política própria e nunca são bearer comum.

## Projetos

- `projectId` e `editionId` são contexto, não autoridade.
- Autorização por projeto requer `projectRole` único e membership vivo nesse escopo.
- Outros papéis da mesma pessoa não são unidos.
- App externo precisa estar ligado ao projeto/edição em uso.
- Headers como `X-Project-Id` podem rotular custo, mas não concedem acesso.

## Consentimentos separados

1. Login confirma a pessoa.
2. Grant autoriza o app a receber tipos/campos aprovados.
3. Link permite que o app sirva o projeto/edição.
4. Membership concede papel e dados coletados pelo projeto.
5. Regra do papel define a operação sobre outra pessoa.
6. Janela define quando a regra está aberta.

Todos os passos necessários devem ser verdadeiros no momento do uso.

## Revogação e sessão

- Apps tratam qualquer 401 como término possível no meio da sessão.
- Descartar tokens, dados protegidos e subscriptions; não repetir o token.
- Mudança de papel/projeto gira o escopo offline e força snapshot novo.
- Serviços Core falham com 503 enquanto a lista de revogação não puder ser usada; não ficam “ready=false” por peer, mas protegem a operação.

## Checklist de nova rota

- audiência e purposes explícitos;
- guard de papel/scope/app/projeto/edição;
- `access: read` impede escrita;
- verificação viva onde o contrato exige;
- 401/403/503 sem vazamento de existência;
- testes negativos de audiência, purpose, papel, app, projeto, edição, revogação e indisponibilidade;
- OpenAPI documenta reasons estáveis;
- nenhuma decisão depende do frontend.
