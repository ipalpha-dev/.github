<p align="center">
  <img src="ipalpha-logo.png" alt="IPAlpha — Igreja Presbiteriana em Alphaville" width="480">
</p>

# IPAlpha — core

Plataforma central da **Igreja Presbiteriana em Alphaville** (Alphaville, SP): pequenos serviços
que os ministérios usam para cuidar das pessoas, sem duplicar cadastros.

## Começar a desenvolver (copie e cole no terminal)

```sh
bash <(curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/bootstrap.sh)
```

Precisa de [chave SSH no GitHub](https://docs.github.com/pt/authentication/connecting-to-github-with-ssh) e ser membro da organização.
Depois: `cd IpAlpha && ./run`.

## Caixa de desenvolvimento (Mailpit)

E-mails e SMS de teste: [https://mailbox-ipalpha.kevyn.com.br](https://mailbox-ipalpha.kevyn.com.br).
Credenciais e instruções: [README privado do notifications-api](https://github.com/ipalpha-dev/notifications-api/blob/master/README.md#developer-inbox-connect-and-sign-in)
(requer acesso ao repositório). Use apenas dados fictícios; a caixa inclui códigos de
verificação. Nunca publique as credenciais nem use esta caixa em produção.

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
| `.github` | Esta página + as ferramentas de desenvolvimento |
