# Secrets e configuração

## Secrets

Incluem credenciais de cliente, tokens, cookies, códigos, senhas, chaves de assinatura/criptografia, provider keys e webhook secrets.

- Nunca commitar, documentar valores, enviar em chat/IA, colocar em frontend/mobile, URL, argumento, imagem ou ConfigMap.
- `.env.example` contém somente vazio, valor sintético local ou referência não secreta.
- Produção usa Secret/secret manager e referência; nunca literal no manifest.
- Respostas que mostram secret são `no-store`, uma única vez, e somem ao sair da tela.
- Agentes não abrem `.env`, Secret, keyring ou credential store para “diagnóstico”. Podem trabalhar com nomes e contratos.

## Geração e rotação

- Use aleatoriedade criptográfica e tamanho definido pelo serviço.
- Rotação exige prova recente quando o owner a executa.
- O valor anterior para imediatamente quando o contrato assim define.
- Incidente presume comprometimento: revogar/rotacionar primeiro, investigar depois.
- Rotação de chave com dados cifrados exige plano de re-encriptação ou consequência documentada.

## Configuração

- Required env falha no boot listando nomes, nunca valores.
- Hosts, issuer, audience, scopes e exchange names não possuem default de produção no código.
- Configuração pública de frontend deve conter apenas identificadores/URLs públicos.
- Ambiente é explícito; valor ausente nunca ativa modo de desenvolvimento ou dado sintético em produção.
- Local força Mailpit; credencial presente não habilita provedor real.

## Arquivos e permissões

- `.env`, caches, dados, backups e saídas de ferramentas ficam ignorados.
- Arquivo local que materializa credencial deve usar permissão restrita quando a plataforma suporta.
- Logs de setup informam apenas nomes configurados, não valores.
