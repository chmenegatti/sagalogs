# SagaLogs

Visualizador local de **sagas** para qualquer projeto **docker compose**.

Lê os logs dos containers direto do Docker, junta as linhas por `transaction_id` e monta a saga
etapa por etapa: qual payload chegou em cada worker, quanto tempo cada um levou, onde parou e
onde deu erro. Sem Loki, sem banco, sem agente nos containers.

```bash
./run.sh --bg      # http://localhost:15001
```

---

## Por quê

Numa arquitetura de saga (gateway → outbox → orquestrador → N workers via fila), um erro
aparece espalhado em vários containers. `docker compose logs -f` mistura tudo e não mostra o
encadeamento. O SagaLogs reconstrói a saga a partir dos logs que as apps já escrevem.

## Telas

### Ao vivo
Stream de todos os containers seguidos, com filtros por container (alt+clique = só este),
tipo de evento, nível e busca de texto. O payload que **chega em cada worker** aparece aberto
como árvore JSON. Clicar numa linha abre o painel com payload, campos e a linha crua.

### Transações
Lista por `transaction_id` (consultas GET sem saga ficam escondidas). Ao escolher uma:

- **Resumo**: status (ok, falhou, falhou e se recuperou, em andamento, parada), onde falhou e a
  mensagem de erro.
- **Rota**: mapa estilo metrô das etapas, com tempo de cada uma e espera entre elas.
- **Raias por aplicação**: linha do tempo por container.
- **Passo a passo**: a saga contada em frases.
- **Eventos**: tabela com todas as linhas da transação.
- **Painel da etapa**: payload de entrada e saída, chamadas externas, mensagem no broker,
  linhas de log.
- **Reprodução**: anima a saga na ordem em que aconteceu.

Falhas de entrega do relay do outbox (`message processing failed`) viram alerta na etapa
Outbox, com o número de tentativas.

## Autoconfiguração

Nada é fixo por nome de app.

| O quê | Como é descoberto |
|---|---|
| Projetos | Label `com.docker.compose.project` de cada container. Seletor no cabeçalho (padrão: todos). Trocar a seleção liga/desliga a leitura na hora. |
| Papel da app | Aprendido pelo que ela loga (abaixo). Sem sinal = worker. |
| Fabricante e nome | Do nome do serviço: `nsxt-nat` → "NSX-T NAT", ícone de rede. |
| Fim da saga | Última etapa é um worker que publicou status diferente de `IN_PROGRESS`. |

Sinais usados para o papel:

| Papel | Linhas que o denunciam |
|---|---|
| worker | `Preparing to execute action`, `Action executed successfully`, `Action execution failed`, `Setting up worker` |
| gateway | `request received` (com `path`), `Relay message added`, logs do relay do outbox |
| orquestrador | `package=middlewares`, `package=handlers.callbacks`, publicação AMQP com `RoutingKey` |

`GET /api/apps` mostra o que foi aprendido e de onde veio (`label`, `logs` ou `padrão`).

### Quando a heurística errar: labels no compose

```yaml
services:
  meu-worker:
    labels:
      sagalogs.role: worker        # gateway | orchestrator | worker
      sagalogs.vendor: vmware      # ícone/fabricante
      sagalogs.label: "VM vSphere" # nome exibido na rota
      sagalogs.final: "true"       # worker que fecha a saga
      sagalogs.ignore: "true"      # não seguir este container
```

## Formato de log esperado

O parser entende três formatos (detectados por linha):

- **JSON / slog JSON**: `{"time":"…","level":"INFO","msg":"…","transaction_id":"…"}`
- **logrus texto**: `time="…" level=info msg="k=v, k=v"`
- **slog texto / logfmt**: `time=… level=INFO msg=… transaction_id=…`

Para montar etapas, os workers precisam logar as mensagens da lib de saga (as da tabela de
papéis), com `transaction_id`, `action`, `resource` e o payload em `state`. Apps que não seguem
esse padrão aparecem no **Ao vivo** normalmente, mas não viram etapas.

## Como funciona

```
docker.sock ──► collector ──► parse/classify ──► ring buffer (memória) ──► SSE  ──► UI ao vivo
   (API do Engine,   (1 follower por    (3 formatos,       (100k linhas,        /api/stream
    sem SDK)          container)         papel aprendido)   índice por tx)
                                                  │
                                                  └──► saga builder ──► /api/saga/{tx} ──► UI transações
```

- **Backend** (`api/`): Go, só biblioteca padrão. Fala com a Docker Engine API pelo unix socket,
  segue os logs com `follow=1&timestamps=1`, demultiplexa stdout/stderr e retoma do último
  timestamp quando o container reinicia. Descobre containers a cada 2 s.
- **Saga builder** (`api/internal/saga`): agrupa eventos em etapas (gateway, outbox,
  orquestrador, workers), liga as etapas pela routing key, calcula esperas, retentativas,
  causa provável do erro, e mascara campos sensíveis (`password`, `token`, `secret`…) nos payloads.
- **UI** (`ui/`): React 19, Vite, Tailwind 4, TanStack Router e React Query, fontes IBM Plex,
  tema claro/escuro.

Tudo fica em memória: reiniciar o SagaLogs zera o histórico (ele relê as últimas
`SAGALOGS_TAIL` linhas de cada container ao subir).

## Rodando

Requisitos: Go 1.26+, [bun](https://bun.sh), acesso a `/var/run/docker.sock`.

```bash
./run.sh                    # build + foreground
./run.sh --bg               # background (log em .sagalogs.log)
./run.sh --stop
REBUILD_UI=1 ./run.sh --bg  # depois de mexer na UI
```

### Variáveis

| Variável | Padrão | |
|---|---|---|
| `SAGALOGS_ADDR` | `:15001` | endereço HTTP |
| `SAGALOGS_PROJECTS` | vazio (todos) | projetos compose, separados por vírgula; também dá para trocar na UI |
| `SAGALOGS_BUFFER` | `100000` | linhas em memória |
| `SAGALOGS_TAIL` | `2000` | linhas lidas por container ao conectar |
| `SAGALOGS_FINAL_APPS` | — | regex dos workers que fecham a saga (alternativa ao label `sagalogs.final`) |
| `SAGALOGS_DOCKER_SOCK` | `/var/run/docker.sock` | socket do Docker |
| `SAGALOGS_DOCKER` | — | `0` desliga a leitura do Docker (útil com replay) |
| `SAGALOGS_REPLAY` | — | arquivo exportado do Grafana/Loki para carregar no buffer |
| `SAGALOGS_UI_DIR` | `ui/dist` | onde está a UI buildada |

### Replay sem Docker

Para ver uma saga de outro ambiente sem disparar nada, exporte os logs do Grafana/Loki
(linhas `ts<TAB>{json do driver json-file}`) e carregue:

```bash
SAGALOGS_DOCKER=0 SAGALOGS_REPLAY=export.txt ./run.sh
```

### Dev da UI

```bash
./run.sh --bg && cd ui && bun run dev   # http://localhost:15002, proxy de /api para a 15001
```

## API

| Rota | |
|---|---|
| `GET /api/projects` / `PUT /api/projects` | projetos compose; PUT `{"projects":[…]}` (vazio = todos) |
| `GET /api/apps` | papel, fabricante e rótulo de cada app, e a origem |
| `GET /api/containers` | containers seguidos e nº de linhas |
| `GET /api/events?q=&container=&kind=&level=&limit=` | linhas do buffer |
| `GET /api/stream` | SSE com as linhas novas |
| `GET /api/tx?q=` | transações |
| `GET /api/tx/{id}` | linhas de uma transação |
| `GET /api/saga/{id}` | saga montada |
| `POST /api/clear` | limpa o buffer |

## Segurança

O SagaLogs lê o socket do Docker (equivale a acesso root na máquina) e expõe os logs sem
autenticação. Use só localmente; não publique a porta.
