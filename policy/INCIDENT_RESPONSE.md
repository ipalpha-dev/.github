# Resposta a incidentes

## Prioridade

Proteger pessoas e interromper o acesso indevido antes de preservar conveniência operacional.

## Ações iniciais

1. Não divulgar detalhes em canal público.
2. Conter: suspender app/entry point, revogar token/client, bloquear rota ou fornecedor conforme o caso.
3. Preservar evidências mínimas e redigidas; não copiar payloads indiscriminadamente.
4. Classificar dados, pessoas, período, sistemas e terceiros afetados.
5. Acionar responsáveis técnico, institucional/LGPD e pastoral conforme impacto.
6. Corrigir, testar negativos, rotacionar e monitorar.
7. Comunicar de forma coordenada, clara e gentil.
8. Fazer revisão pós-incidente sem culpabilizar pessoas; converter aprendizado em gate.

## Situações mínimas

- secret/token/código exposto: revogar/rotacionar imediatamente; remover do histórico não basta;
- mensagem enviada à pessoa errada: parar lote/provedor, preservar IDs e avaliar contato exposto;
- dado pessoal/saúde/documento acessado: revogar sessão/app, consultar access logs e delimitar leituras;
- biometria/foto exposta: isolar serviço/URL, interromper indexação e aplicar retenção/exclusão;
- IA/processador: bloquear egress/chave, identificar classes enviadas e política de retenção do fornecedor;
- preview com dado real: congelar acesso, destruir cópias após evidência mínima e investigar origem;
- dependência/artefato comprometido: bloquear deploy, identificar digest/SBOM e reconstruir de fonte confiável.

## Agentes de IA

Agentes podem ajudar a preparar diagnóstico e correção com dados sintéticos, mas não abrem secrets, consultam dados reais, notificam titulares, executam destruição ou decidem severidade institucional autonomamente.
