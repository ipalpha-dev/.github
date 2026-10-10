# Entrega segura e gates

## Fluxo obrigatório

1. branch de feature;
2. classificação de risco e perfil atualizado;
3. checks locais;
4. pull request;
5. gates automáticos;
6. aprovação técnica e, quando aplicável, pastoral/dados;
7. artefato imutável;
8. aprovação de ambiente;
9. deploy do mesmo artefato aprovado;
10. verificação e rollback disponível.

Branch principal não recebe push direto de pessoas ou IA. Ferramentas de publish podem preparar commit/plano, mas não contornar PR, proteção de branch ou ambiente protegido.

## Gates mínimos por PR

- lint, typecheck, testes e build;
- lockfile reproduzível (`npm ci`/equivalente congelado);
- secret scan do diff e histórico periódico;
- SAST e dependency/OSV scan;
- testes negativos de autorização quando rota/política muda;
- validação de contrato/OpenAPI;
- validação de política/perfil e links de documentação;
- scan de imagem/IaC quando aplicável;
- fixture sintética e ausência de dado real;
- revisão CODEOWNERS para área sensível.

## Supply chain

- Dependências e Actions pinadas; atualização revisada.
- SBOM para artefatos de produção.
- Imagem sem tag mutável no manifest final; registrar digest.
- Build não recebe secret desnecessário e não o incorpora em camada/bundle.
- Assinatura/proveniência é o alvo para releases de produção.

## Infraestrutura

Policy-as-code bloqueia Secret literal, container privilegiado, imagem `latest`, exposição de métricas/biometria, ausência de limites e preview conectado a produção. Exceção vigente deve ser referenciada no código/IaC.

## Deploy

- Operação destrutiva e migração são etapa separada com confirmação humana.
- Não rebuildar entre aprovação e deploy.
- Verificar rollout, digest e probes; probe não prova autorização, então executar smoke negativo com dados sintéticos.
- Rollback não restaura permissões revogadas nem secrets antigos automaticamente.

## Riscos que exigem revisão adicional

`auth`, criptografia, scopes, dados de crianças/saúde, biometria, IA, endpoint público, retenção/exclusão, nova integração externa e infraestrutura de produção.
