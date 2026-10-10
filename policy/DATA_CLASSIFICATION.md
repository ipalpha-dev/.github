# Classificação e manuseio de dados

Dados podem pertencer a mais de uma classe; aplique a mais restritiva. “Não sensível” em um contrato técnico não significa “não pessoal” para LGPD.

| Classe | Exemplos | Regra padrão |
|---|---|---|
| pública | documentação publicada, diretório aprovado, identidade visual | pode ser publicada após revisão |
| interna | configuração não secreta, IDs opacos, métricas agregadas | somente necessidade operacional |
| pessoal | nome, foto, idioma, vínculo, personId associável | uso por finalidade; não cachear em consumidores |
| sensível | contato, nascimento, endereço, documento, saúde, criança, família | leitura no dono, auditoria e retenção mínima |
| biométrica | imagem facial usada para reconhecimento, embedding | aprovação específica, isolamento e descarte |
| credencial | senha, token, código, cookie, secret, chave | nunca em docs, logs, browser storage ou chat |

## Regras por ambiente

| Ambiente | Dados pessoais/sensíveis/biométricos |
|---|---|
| produção | somente finalidade aprovada e controles completos |
| preview | proibidos; fixtures sintéticas `.test` |
| CI/teste | proibidos; geradores determinísticos sintéticos |
| local | proibidos; nunca baixar dump de produção |

## Armazenamento

- O `persons-api` é o registro canônico de pessoas. Outros serviços guardam somente relações por ID e seus dados de domínio explicitamente aprovados.
- Consumidores não persistem resposta de pessoas, fotos, contatos, documentos ou saúde para “performance”.
- Dados offline exigem perfil aprovado, criptografia, escopo por sessão/papel/projeto, expiração e limpeza.
- IDs associados a atividade humana continuam sendo dados pessoais internos, mesmo sem nome.

## Transferência

- HTTPS em trânsito fora do processo/cluster conforme arquitetura aprovada.
- Eventos carregam a projeção mínima. Prefira IDs/invalidações a snapshots.
- Contato para mensagens é resolvido no momento do envio; apps não recebem listas de telefones/e-mails.
- Dados sensíveis e biométricos não vão a IA externa sem aprovação registrada.

## Exibição e exportação

- A UI não infere ausência de dado quando a resposta pode significar falta de permissão/consentimento.
- Exportações são rotas próprias, auditadas e nunca um reaproveitamento de listagem interna.
- Arquivos pessoais usam `Cache-Control: no-store`, content type validado e autorização a cada leitura.

## Revisão obrigatória

Nova categoria, campo, cópia, índice de busca, cache, exportação ou integração deve atualizar o perfil de segurança e responder: finalidade, dono, sujeitos, leitores, escritores, retenção, exclusão, logs, backups, ambientes e processadores.
