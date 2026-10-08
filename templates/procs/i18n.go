package main

var lang = "pt-BR"

var messages = map[string]map[string]string{
	"pt-BR": {
		"browser_on":     "abre no próximo run",
		"browser_off":    "não abre no próximo run",
		"browser_failed": "não foi possível abrir ou salvar a preferência do navegador",
		"dependencies":   "Infraestrutura",
		"processes":      "PROCESSOS",
		"focus":          "foco",
		"follow":         "seguindo",
		"section_api":    "── Serviços do core ──",
		"section_web":    "── Frontends ──",
		"section_apps":   "── Apps ──",
		"running":        "rodando",
		"starting":       "iniciando",
		"waiting":        "aguardando dependências",
		"exited":         "encerrado",
		"exit":           "saída",
		"stopped":        "parado",
		"deps":           "depende de",
		"cycle":          "ciclo",
		"failed":         "com falha",
		"no_output":      "(sem saída ainda)",
		"loading":        "Abrindo o painel…",
		"all_stopped":    "Todos os processos foram parados.",
		"shutting_down":  "encerrando…",
		"stopping":       "parando",
		"stopping_all":   "parando todos os processos…",
		"all_done":       "todos os processos parados",
		"log_focus":      "foco no log (↑/↓ rola)",
		"proc_focus":     "foco nos processos (↑/↓ seleciona)",
		"restart":        "reiniciando",
		"start":          "iniciando",
		"stop":           "parando",
		"open":           "abrindo",
		"no_frontend":    "sem frontend",
		"cleared":        "log limpo",
		"help":           "↑/↓ seleciona ou rola  ·  Tab/←→ foco  ·  s inicia/para e lembra apps  ·  r reinicia  ·  o abre e lembra  ·  x para e esquece  ·  c limpa  ·  arraste copia  ·  y copia tudo  ·  f segue  ·  PgUp/PgDn  ·  q sai",
		"wait_deps":      "aguardando dependências",
		"uses":           "usa",
		"not_ready":      "no ar · ainda não pronto (/ready 503)",
		"no_port":        "porta desconhecida, sem espera",
		"is_up":          "está no ar",
		"timed_out":      "tempo esgotado aguardando",
		"starting_any":   "iniciando mesmo assim",
		"cycle_note":     "ciclo de dependência com",
		"start_no_wait":  "iniciando sem esperar",
		"start_cancel":   "início cancelado",
		"start_error":    "erro ao iniciar",
		"title":          " IPAlpha run ",
		"select_on":      "modo seleção: arraste com o mouse para copiar (m volta)",
		"select_off":     "modo seleção desligado",
		"copied":         "log copiado",
		"lines":          "linhas",
		"copy_fail":      "não foi possível copiar",
	},
	"en-US": {
		"browser_on":     "opens on next run",
		"browser_off":    "won't open on next run",
		"browser_failed": "could not open or save the browser preference",
		"dependencies":   "Infrastructure",
		"processes":      "PROCESSES",
		"focus":          "focus",
		"follow":         "follow",
		"section_api":    "── Core services ──",
		"section_web":    "── Frontends ──",
		"section_apps":   "── Apps ──",
		"running":        "running",
		"starting":       "starting",
		"waiting":        "waiting for deps",
		"exited":         "exited",
		"exit":           "exit",
		"stopped":        "stopped",
		"deps":           "deps",
		"cycle":          "cycle",
		"failed":         "failed",
		"no_output":      "(no output yet)",
		"loading":        "Starting process panel…",
		"all_stopped":    "Stopped all processes.",
		"shutting_down":  "shutting down…",
		"stopping":       "stopping",
		"stopping_all":   "stopping all processes…",
		"all_done":       "all processes stopped",
		"log_focus":      "log focus (↑/↓ scroll)",
		"proc_focus":     "process focus (↑/↓ select)",
		"restart":        "restart",
		"start":          "start",
		"stop":           "stop",
		"open":           "opening",
		"no_frontend":    "no frontend",
		"cleared":        "log cleared",
		"help":           "↑/↓ select or scroll  ·  Tab/←→ focus  ·  s start/stop and remember apps  ·  r restart  ·  o open and remember  ·  x stop and forget  ·  c clear  ·  drag to copy  ·  y copy all  ·  f follow  ·  PgUp/PgDn  ·  q quit",
		"wait_deps":      "waiting for deps",
		"uses":           "uses",
		"not_ready":      "up · not ready yet (/ready 503)",
		"no_port":        "no port known, skip wait",
		"is_up":          "is up",
		"timed_out":      "timed out waiting for",
		"starting_any":   "starting anyway",
		"cycle_note":     "dependency cycle with",
		"start_no_wait":  "starting without waiting",
		"start_cancel":   "start cancelled",
		"start_error":    "start error",
		"title":          " IPAlpha run ",
		"select_on":      "select mode: drag with the mouse to copy (m to exit)",
		"select_off":     "select mode off",
		"copied":         "log copied",
		"lines":          "lines",
		"copy_fail":      "copy failed",
	},
	"es": {
		"section_api": "── Servicios del core ──", "section_web": "── Frontends ──", "section_apps": "── Apps ──",
		"browser_on": "se abre en la próxima ejecución", "browser_off": "no se abre en la próxima ejecución", "browser_failed": "no se pudo abrir o guardar la preferencia del navegador",
		"help": "↑/↓ seleccionar o desplazar · Tab/←→ foco · s iniciar/parar y recordar apps · r reiniciar · o abrir y recordar · x parar y olvidar · c limpiar · arrastrar para copiar · y copiar todo · f seguir · PgUp/PgDn · q salir",
	},
	"fr": {
		"section_api": "── Services du core ──", "section_web": "── Frontends ──", "section_apps": "── Apps ──",
		"browser_on": "s’ouvre au prochain lancement", "browser_off": "ne s’ouvre pas au prochain lancement", "browser_failed": "impossible d’ouvrir ou d’enregistrer la préférence du navigateur",
		"help": "↑/↓ sélectionner ou défiler · Tab/←→ focus · s démarrer/arrêter et mémoriser les apps · r redémarrer · o ouvrir et mémoriser · x arrêter et oublier · c effacer · glisser pour copier · y tout copier · f suivre · PgUp/PgDn · q quitter",
	},
	"de": {
		"section_api": "── Core-Dienste ──", "section_web": "── Frontends ──", "section_apps": "── Apps ──",
		"browser_on": "öffnet beim nächsten Start", "browser_off": "öffnet nicht beim nächsten Start", "browser_failed": "Browser konnte nicht geöffnet oder die Einstellung gespeichert werden",
		"help": "↑/↓ auswählen oder scrollen · Tab/←→ Fokus · s Apps starten/stoppen und merken · r neu starten · o öffnen und merken · x stoppen und vergessen · c leeren · ziehen zum Kopieren · y alles kopieren · f folgen · PgUp/PgDn · q beenden",
	},
}

func setLang(l string) {
	switch l {
	case "en", "en-US":
		lang = "en-US"
	case "es", "fr", "de":
		lang = l
	default:
		lang = "pt-BR"
	}
}

func tr(key string) string {
	if s, ok := messages[lang][key]; ok {
		return s
	}
	if s, ok := messages["pt-BR"][key]; ok {
		return s
	}
	return key
}
