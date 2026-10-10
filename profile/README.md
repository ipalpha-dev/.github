<p align="center">
  <img src="ipalpha-logo.png" alt="IPAlpha — Igreja Presbiteriana em Alphaville" width="480">
</p>

# IPAlpha — core

Plataforma central da **Igreja Presbiteriana em Alphaville** (Alphaville, SP): pequenos serviços
que os ministérios usam para cuidar das pessoas, sem duplicar cadastros.

## Começar a desenvolver (copie e cole no terminal)

macOS, Linux, WSL:

```sh
curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.ps1 | iex
```

Precisa ser membro da organização. O setup instala o que faltar e entra no GitHub pelo navegador
(ou usa sua chave SSH). Depois: `cd IpAlpha` e `./run` (`.\run` no Windows). Algo deu errado? `./doctor`.

## Repositórios

| Repositório | Para quê |
|---|---|
| `shared-js` | Biblioteca comum (`@ipalpha/shared-js`) |
| `auth-api` | Login (código por SMS, passkeys) e tokens |
| `persons-api` | Cadastro de pessoas, registro de acesso (LGPD) |
| `organizations-api` | Estrutura da igreja (organograma) |
| `projects-api` | Projetos e aplicativos conectados |
| `notifications-api` | Envio de SMS e modelos de mensagem |
| `deployment` | Manifestos Kubernetes |
| `.github` | Esta página + a ferramenta `ipalpha` (setup, run, publish, feature, doctor) |
