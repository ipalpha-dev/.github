# Exceções de segurança

Uma exceção é temporária e explícita; não é precedente nem autorização genérica.

## Registro obrigatório

Use `templates/security-exception.md` com:

- ID e estado;
- regra exata;
- serviço/ambiente/escopo;
- necessidade e alternativas avaliadas;
- classes de dados;
- risco e pior impacto;
- controles compensatórios;
- responsável técnico e institucional;
- aprovadores;
- criação, expiração e revisão;
- plano de remoção;
- evidência de testes/monitoramento.

## Limites

- Toda exceção expira; sem renovação, o gate volta a bloquear.
- Nunca autoriza segredo em repositório/frontend/log, dado real em preview, bypass não auditado ou aceitação de token não verificado.
- Comentário, chat, issue, README legado ou “sempre foi assim” não vale como exceção.
- Código/IaC afetado referencia apenas o ID, nunca dados sensíveis do registro.
- Renovação reavalia risco; não copia automaticamente a justificativa anterior.

## Processo

1. Agente prepara proposta sem aplicar a violação.
2. Revisor técnico valida controles.
3. Responsável pelos dados/finalidade aprova quando aplicável.
4. Gate recebe allowlist limitada a caminho/regra e data.
5. Monitoramento e tarefa de remoção são criados.
