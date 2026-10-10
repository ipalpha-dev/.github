# Checklist de segurança da mudança

- [ ] Li a política global, o AGENTS e o perfil do serviço.
- [ ] Classifiquei dados e atualizei o inventário quando necessário.
- [ ] A mudança não amplia acesso; se amplia, possui aprovação tipada.
- [ ] Autorização está no backend e possui testes negativos.
- [ ] Não adicionei persistência/cache/log de dados pessoais ou secrets.
- [ ] Local, testes e preview usam somente dados sintéticos.
- [ ] Integrações externas/IA possuem registro aprovado.
- [ ] Retenção, exclusão, backup e auditoria estão definidos.
- [ ] Rotas públicas, scopes, eventos e peers foram atualizados no perfil.
- [ ] Instalação reproduzível, testes, typecheck/build e gates passaram.
- [ ] Operações destrutivas estão fora da automação da IA e exigem aprovação humana.
