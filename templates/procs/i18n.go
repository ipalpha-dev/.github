package main

var lang = "pt-BR"

var messages = map[string]map[string]string{
	"pt-BR": {
		"dependencies":  "Infraestrutura",
		"processes":     "PROCESSOS",
		"focus":         "foco",
		"follow":        "seguindo",
		"section_api":   "── Serviços ──",
		"section_web":   "── Web ──",
		"running":       "rodando",
		"starting":      "iniciando",
		"waiting":       "aguardando dependências",
		"exited":        "encerrado",
		"exit":          "saída",
		"stopped":       "parado",
		"deps":          "depende de",
		"cycle":         "ciclo",
		"failed":        "com falha",
		"no_output":     "(sem saída ainda)",
		"loading":       "Abrindo o painel…",
		"all_stopped":   "Todos os processos foram parados.",
		"shutting_down": "encerrando…",
		"stopping":      "parando",
		"stopping_all":  "parando todos os processos…",
		"all_done":      "todos os processos parados",
		"log_focus":     "foco no log (↑/↓ rola)",
		"proc_focus":    "foco nos processos (↑/↓ seleciona)",
		"restart":       "reiniciando",
		"start":         "iniciando",
		"stop":          "parando",
		"open":          "abrindo",
		"no_frontend":   "sem frontend",
		"cleared":       "log limpo",
		"help":          "↑/↓ seleciona ou rola  ·  Tab/←→ foco  ·  s inicia/para  ·  r reinicia  ·  o abre frontend  ·  c limpa  ·  y copia log  ·  m seleciona  ·  f segue  ·  PgUp/PgDn  ·  q sai",
		"wait_deps":     "aguardando dependências",
		"no_port":       "porta desconhecida, sem espera",
		"is_up":         "está no ar",
		"timed_out":     "tempo esgotado aguardando",
		"starting_any":  "iniciando mesmo assim",
		"cycle_note":    "ciclo de dependência com",
		"start_no_wait": "iniciando sem esperar",
		"start_cancel":  "início cancelado",
		"start_error":   "erro ao iniciar",
		"title":         " IPAlpha run ",
		"select_on":     "modo seleção: arraste com o mouse para copiar (m volta)",
		"select_off":    "modo seleção desligado",
		"copied":        "log copiado",
		"lines":         "linhas",
		"copy_fail":     "não foi possível copiar",
	},
	"en-US": {
		"dependencies":  "Infrastructure",
		"processes":     "PROCESSES",
		"focus":         "focus",
		"follow":        "follow",
		"section_api":   "── Services ──",
		"section_web":   "── Web ──",
		"running":       "running",
		"starting":      "starting",
		"waiting":       "waiting for deps",
		"exited":        "exited",
		"exit":          "exit",
		"stopped":       "stopped",
		"deps":          "deps",
		"cycle":         "cycle",
		"failed":        "failed",
		"no_output":     "(no output yet)",
		"loading":       "Starting process panel…",
		"all_stopped":   "Stopped all processes.",
		"shutting_down": "shutting down…",
		"stopping":      "stopping",
		"stopping_all":  "stopping all processes…",
		"all_done":      "all processes stopped",
		"log_focus":     "log focus (↑/↓ scroll)",
		"proc_focus":    "process focus (↑/↓ select)",
		"restart":       "restart",
		"start":         "start",
		"stop":          "stop",
		"open":          "opening",
		"no_frontend":   "no frontend",
		"cleared":       "log cleared",
		"help":          "↑/↓ select or scroll  ·  Tab/←→ focus  ·  s start/stop  ·  r restart  ·  o open frontend  ·  c clear  ·  y copy log  ·  m select  ·  f follow  ·  PgUp/PgDn  ·  q quit",
		"wait_deps":     "waiting for deps",
		"no_port":       "no port known, skip wait",
		"is_up":         "is up",
		"timed_out":     "timed out waiting for",
		"starting_any":  "starting anyway",
		"cycle_note":    "dependency cycle with",
		"start_no_wait": "starting without waiting",
		"start_cancel":  "start cancelled",
		"start_error":   "start error",
		"title":         " IPAlpha run ",
		"select_on":     "select mode: drag with the mouse to copy (m to exit)",
		"select_off":    "select mode off",
		"copied":        "log copied",
		"lines":         "lines",
		"copy_fail":     "copy failed",
	},
}

func setLang(l string) {
	switch l {
	case "en", "en-US":
		lang = "en-US"
	default:
		lang = "pt-BR"
	}
}

func tr(key string) string {
	if s, ok := messages[lang][key]; ok {
		return s
	}
	return key
}
