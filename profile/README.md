<p align="center">
  <img src="ipalpha-logo.png" alt="IPAlpha — Igreja Presbiteriana em Alphaville" width="480">
</p>

# IPAlpha — core

Plataforma central da **Igreja Presbiteriana em Alphaville** (Alphaville, SP): pequenos serviços
que os ministérios usam para cuidar das pessoas, sem duplicar cadastros.

| Repositório | Para quê |
|---|---|
| `develop` | Ferramentas: `setup`, `run`, `pull`, `publish` |
| `shared-js` | Biblioteca comum (`@ipalpha/shared-js`) |
| `auth-api` | Login (código por SMS, passkeys) e tokens |
| `person-api` | Cadastro de pessoas, registro de acesso (LGPD) |
| `organization-api` | Estrutura da igreja (organograma) |
| `projects-api` | Projetos e aplicativos conectados |
| `notification-api` | Envio de SMS e modelos de mensagem |
| `deployment` | Manifestos Kubernetes |
