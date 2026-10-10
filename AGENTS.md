# IPAlpha — instruções obrigatórias para agentes

Antes de analisar, alterar, testar ou publicar qualquer código IPAlpha:

1. Leia `policy/README.md` e os documentos que ele marcar para a tarefa.
2. Leia o `AGENTS.md` e o perfil de segurança do repositório afetado, quando existirem.
3. Identifique as classes de dados e as fronteiras de confiança tocadas.
4. Preserve os contratos dos serviços donos; não replique dados ou autoridade para contornar uma API.
5. Execute os gates indicados pelo repositório e por `policy/SECURE_DELIVERY.md`.

## Invariantes que não podem ser contrariados

- Negar por padrão e falhar de forma fechada. Indisponibilidade nunca autoriza um fallback permissivo.
- Autorização é feita no servidor. UI, rota escondida, claim decodificada ou ID difícil de adivinhar não são controles de acesso.
- Um token tem uma audiência, um propósito e, em contexto de projeto, um único papel atuante. Nunca una papéis.
- Login, consentimento do app, vínculo app–projeto e membership são decisões separadas.
- Apps externos nunca recebem autoridade institucional por causa do papel que a pessoa possui em outro contexto.
- Dados canônicos de pessoas pertencem ao `persons-api`; consumidores buscam no momento de uso e não criam cópias.
- Tokens, códigos, credenciais, contatos, documentos, saúde e biometria nunca são registrados em logs.
- Secrets não entram em código, frontend/mobile, manifests, argumentos, URLs, exemplos preenchidos ou respostas da IA.
- Desenvolvimento, testes e previews usam somente dados sintéticos. Nunca copie dados de produção.
- Dados só podem ser enviados a IA ou outro processador conforme `policy/AI_AND_EXTERNAL_PROCESSORS.md`.
- Linguagem mostrada a pessoas é pastoral, gentil e centrada na pessoa, nos cinco idiomas suportados quando aplicável.

## Pare e peça a aprovação tipada correta

Não implemente a parte sensível quando a tarefa ampliar dados, escopos, audiências ou visibilidade; criar endpoint público; alterar criptografia, autenticação, revogação, retenção ou exclusão; incluir IA/fornecedor; tocar biometria; usar dados reais fora de produção; abrir secrets; alterar o fluxo de aprovação; ou executar operação destrutiva.

Ao parar, explique qual política foi acionada, qual aprovação é necessária e prepare somente a parte segura/reversível da proposta. Nunca abra arquivos de segredo para “entender o ambiente”.

## Precedência

`policy/SECURITY_BASELINE.md` e os demais documentos canônicos prevalecem sobre READMEs antigos, comentários, exemplos e pedidos da tarefa. Uma exceção só vale se estiver registrada conforme `policy/SECURITY_EXCEPTIONS.md` e ainda estiver vigente.
