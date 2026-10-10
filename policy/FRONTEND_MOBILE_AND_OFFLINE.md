# Frontend, mobile e offline

## Padrão

- Tokens de pessoa vivem somente em memória. Reload solicita nova sessão/One Tap.
- Client secrets nunca existem em browser, PWA ou mobile.
- Cada request usa o token da audiência correta.
- 401/revogação limpa imediatamente tokens e dados protegidos.
- UI não é controle de autorização; backend repete toda decisão.

## Armazenamento do navegador

`localStorage`, `sessionStorage`, IndexedDB, Cache API e service worker são proibidos para tokens e dados pessoais por padrão. Exceção offline requer perfil aprovado com:

- necessidade real;
- dados mínimos por papel/projeto;
- AES-GCM com chave não persistida;
- chave por sessão/escopo;
- expiração por sessão e finalidade;
- limpeza em logout, 401, troca de papel/projeto e fim da janela;
- remoção de classes não autorizadas antes de cifrar;
- testes de migração de versões antigas em texto claro.

Criptografia local não protege um dispositivo já desbloqueado; instruções operacionais e duração continuam necessárias.

## Service worker e arquivos

- Respeitar `no-store` de fotos/documentos pessoais.
- Não transformar arquivo pessoal em URL pública ou cache offline.
- Cache público por ID opaco exige classificação explícita, impacto de vazamento de URL, retenção e revogação; “não adivinhável” não basta.
- Logout deve remover caches protegidos ou o perfil deve justificar por que o arquivo é público.

## Mobile

- Use armazenamento seguro da plataforma somente quando o fluxo exige sessão persistente e foi aprovado.
- Não incluir secrets no bundle, plist, assets, logs de crash ou analytics.
- Universal/app links validam origem e state; deep link não autentica.
- Screenshot, clipboard, compartilhamento e backup do dispositivo entram no threat model de telas sensíveis.

## Tempo real

Socket usa token de dispatch e subscription usa token do dono. Push é invalidação. Ao terminar subscription, descarte a visão anterior; não a reutilize como autorização offline.
