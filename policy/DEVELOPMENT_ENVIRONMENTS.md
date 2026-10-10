# Ambientes de desenvolvimento, teste e preview

## Regra absoluta

Local, teste, CI e preview usam somente pessoas, contatos, arquivos e eventos sintéticos. Nunca copie banco, arquivo, foto, mensagem, log ou backup de produção.

## Identidade sintética

- Domínios `.test`, números reservados e nomes marcados como exemplo.
- Fixtures determinísticas e documentadas.
- Nenhum contato roteável para pessoa real.
- Casos de saúde/família inventados e com linguagem gentil.

## Notificações

- Local usa Mailpit para e-mail e SMS, mesmo se alguma credencial existir no ambiente.
- Erro de captura nunca faz fallback para provedor real.
- Preview usa mailbox sintética e credenciais próprias limitadas.
- Smoke test de produção não envia código ou broadcast real por conveniência.

## Isolamento

- Cada workspace/feature possui banco, volumes, namespace, clientes e portas próprios.
- Preview não alcança banco/volume/secret de produção.
- Secret de preview é limitado por orçamento/finalidade e não é compartilhado com produção.
- Expiração remove namespace e dados; extensão é auditável.

## Plataformas

- macOS: Docker Desktop ou Apple container conforme suporte documentado.
- Linux: Debian/Ubuntu com Docker Engine + Compose v2; instalador deve detectar pacote/gerenciador ou orientar comando exato.
- Windows: Ubuntu WSL2 é o ambiente suportado enquanto não existir instalador PowerShell validado. Git Bash/PowerShell nativo não devem ser anunciados como suportados.

## Setup seguro

- Bootstrap deve usar transporte autenticado e, idealmente, versão/checksum pinado.
- Ferramentas faltantes geram instrução específica por SO.
- Setup não imprime secrets e cria arquivos locais restritos.
- `./run` não abre interfaces de infraestrutura fora de loopback salvo necessidade explícita.
