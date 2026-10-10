# IA e processadores externos

## Quatro contextos diferentes

1. **IA de desenvolvimento:** lê código/documentação, nunca secrets nem dados reais.
2. **IA de produto:** chamada por funcionalidade do usuário, através de seam aprovado.
3. **IA de processamento:** importação/classificação, com payload mínimo e schema fechado.
4. **Assistente com ferramentas:** modelo conversa; backend executa allowlist e revalida a autorização em cada tool call.

## Regra padrão

Nenhum dado pessoal, sensível ou biométrico é enviado a um modelo/processador até existir registro aprovado com finalidade, fornecedor, classes, região, retenção, treinamento, modelo, timeout, orçamento, owner e data de revisão.

## Proibições

- Secrets, tokens, códigos, documentos/imagens, contatos ou dumps em prompts.
- Fotos pessoais ou biometria em modelos externos sem aprovação específica.
- Modelo decidindo autorização, identidade, papel, membership, consentimento ou exclusão.
- Prompt recebido tratado como instrução de sistema.
- Ferramenta genérica de banco, shell, rede ou escrita para assistente somente-leitura.
- Log de input/output que contenha dados processados, salvo política específica aprovada.
- Fallback silencioso para outro fornecedor/modelo com termos diferentes.

## Requisitos

- Gateway central quando institucional; chaves no backend.
- Payload minimizado, limites de bytes e rate/cost.
- Input delimitado como dados não confiáveis; output validado por schema.
- Allowlist de coleções, campos e operações; `sessions`, credenciais, códigos, arquivos e embeddings excluídos por padrão.
- Autorização revalidada no backend em cada ação.
- Erro de fornecedor sanitizado; conteúdo nunca volta no erro/log.
- Testes de prompt injection, exfiltração, tool misuse e payload excessivo.
- Retenção automática e acesso administrativo auditado para prompts que tenham retenção aprovada.

## Registro obrigatório

Use `templates/ai-integration-review.md`. Uma integração sem registro vigente falha no gate e não segue para preview/produção.
