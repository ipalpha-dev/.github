# Ciclo de vida dos dados

Cada perfil de serviço mantém inventário com: coleção/arquivo, classe, finalidade, sujeitos, readers/writers, fonte canônica, retenção, TTL/job, backup, exportação, exclusão, eventos e processadores.

## Criação e alteração

- Validar antes de persistir e registrar origem/ator quando necessário.
- Membership e consentimentos crescem/reduzem somente pelos fluxos donos.
- Mudança concorrente usa versão/CAS; não sobrescrever silenciosamente.
- Importação é idempotente e não apaga informação sensível existente por ausência na planilha.

## Retenção

- Prazo definido, não “para sempre” por conveniência.
- Evidência legal/auditoria separada do dado operacional e minimizada.
- TTL e jobs possuem teste; falha de limpeza é observável.
- Previews inteiros expiram automaticamente.

## Backups

- Allowlist de coleções/campos; sessões, credenciais, códigos e temporários excluídos.
- Backup herda classificação e controle de acesso.
- Restauração aceita somente versão/formato compatível e é testada.
- Banco e arquivos relacionados devem ser consistentes.

## Exclusão e esquecimento

- Owner coordena remoção de registros, arquivos e cópias derivadas aprovadas.
- Relações por ID e eventos de delete permitem consumidores remover referências.
- Dados biométricos e fotos seguem retenção específica.
- Índices, caches, queues, exports e backups são considerados no plano.
- Operação destrutiva exige runbook separado e aprovação humana; nunca é decisão autônoma da IA.

## Arquivamento

Arquivado significa somente leitura, não invisível nem excluído. Backend impõe o bloqueio; frontend apenas comunica. Reativação e acesso histórico obedecem política explícita.
