# Baseline de segurança

## Princípios obrigatórios

1. **Negar por padrão.** Ausência, ambiguidade, timeout, cache não carregado ou peer indisponível não concedem acesso.
2. **Menor privilégio.** Solicite e emita apenas audiência, purpose, scope, dados e duração necessários.
3. **Decisão no dono.** O serviço dono do recurso autoriza cada operação com estado atual; consumidores não reproduzem a decisão.
4. **Separação de decisões.** Identidade, consentimento do app, vínculo app–projeto, membership, papel atuante e janela de acesso não se substituem.
5. **Uma atuação por vez.** Um token de projeto atua com um único papel. Papéis não são unidos para formar permissão.
6. **Propósito antes de papel.** Um papel privilegiado em token externo não cria autoridade institucional.
7. **Revogação a qualquer momento.** Consumidores encerram a sessão e descartam dados após 401; nunca repetem o mesmo token.
8. **Minimização.** Colete, leia, envie, armazene e retenha somente o necessário para a finalidade aprovada.
9. **Dados sintéticos fora de produção.** Local, testes, CI e previews não recebem dados de membros.
10. **Defesa verificável.** Regra crítica precisa de teste/gate quando tecnicamente possível.

## Fronteiras de confiança

- Frontend e mobile são ambientes não confiáveis.
- Parâmetros, headers, claims opcionais, nomes de arquivo, MIME declarado, HTML, planilhas, prompts e eventos externos são entrada não confiável.
- IDs opacos e URLs não substituem autenticação.
- Rede interna não substitui autenticação, exceto serviço deliberadamente sem auth que esteja tecnicamente inacessível fora do cluster e possua gate de infraestrutura.
- IA e fornecedores são processadores externos, não autoridades.

## Proibições universais

- Bypass temporário de auth em código compartilhado ou produção.
- Compatibilidade legada que aceite token, audiência, issuer, purpose ou claim mais fraco.
- Autorização baseada apenas em UI, rota escondida, nome de papel vindo do cliente ou JWT decodificado sem verificação.
- Cópia de dados canônicos para evitar chamada ao serviço dono.
- Logging de bodies/headers completos em rotas sensíveis.
- “Fallback” para provedor real em ambiente de desenvolvimento.
- Operação destrutiva decidida ou executada autonomamente por agente de IA.

## Tratamento de falhas

- **401:** sessão/token terminou; descartar tokens e dados protegidos, fechar canais e voltar ao login com mensagem gentil.
- **403:** não tentar outra identidade, papel ou scope para contornar; mostrar indisponibilidade para o perfil.
- **409:** resolver concorrência com versão/idempotência; não forçar overwrite.
- **429:** respeitar `Retry-After`; não paralelizar para contornar limite.
- **503:** manter indisponível e tentar novamente com backoff quando o contrato permitir; nunca ampliar acesso.

## Definição de pronto

Uma mudança sensível só está pronta quando possui autorização no servidor, testes positivos e negativos, logs redigidos, documentação/perfil atualizado, retenção definida e gates verdes.
