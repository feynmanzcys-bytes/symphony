// The few messages the mod shows itself, in the languages the office page offers.
// The session's language comes from SYMPHONY_LANG (the bridge passes the page's language when the
// page starts a session), then the POSIX locale (the first of LC_ALL, LC_MESSAGES and LANG that is
// set), and English otherwise.

const MESSAGES = {
  en: {
    unreachable: "symphony: can't reach the bridge ({bridge}); approvals stay in the terminal for now",
    started: 'symphony: started the bridge ({bridge}) in the background; run symphony to open the office',
    badToken: "symphony: the bridge doesn't know this SYMPHONY_TOKEN; approvals stay in the terminal",
    notBridge: "symphony: what answers at {bridge} isn't a symphony bridge this session can trust, or it's older than this plugin (update symphony); approvals stay in the terminal",
    insecure: 'symphony: SYMPHONY_BRIDGE ({bridge}) must be on this machine or use https; approvals stay in the terminal',
    busy: 'symphony: this account already has a session online; approvals go to the terminal for now',
    allowed: 'symphony: allowed in the web page',
    denied: 'symphony: denied in the web page',
  },
  'zh-CN': {
    unreachable: 'symphony: 连不上 bridge（{bridge}），审批先走终端',
    started: 'symphony: 已在后台启动 bridge（{bridge}），运行 symphony 打开办公室',
    badToken: 'symphony: bridge 不认这个 SYMPHONY_TOKEN，审批仍走终端',
    notBridge: 'symphony: {bridge} 上应答的不是本会话能信任的 symphony bridge，或者它比这个插件旧（请更新 symphony），审批仍走终端',
    insecure: 'symphony: SYMPHONY_BRIDGE（{bridge}）必须在本机或使用 https，审批仍走终端',
    busy: 'symphony: 这个账号已经有会话在线，审批先走终端',
    allowed: 'symphony: 网页端允许',
    denied: 'symphony: 网页端拒绝',
  },
  'zh-TW': {
    unreachable: 'symphony: 連不上 bridge（{bridge}），核准暫時改在終端機進行',
    started: 'symphony: 已在背景啟動 bridge（{bridge}），執行 symphony 即可開啟辦公室',
    badToken: 'symphony: bridge 不認得這個 SYMPHONY_TOKEN，核准仍在終端機進行',
    notBridge: 'symphony: {bridge} 上回應的不是這個工作階段能信任的 symphony bridge，或是它比這個外掛舊（請更新 symphony），核准仍在終端機進行',
    insecure: 'symphony: SYMPHONY_BRIDGE（{bridge}）必須在這台電腦上或使用 https，核准仍在終端機進行',
    busy: 'symphony: 這個帳號已經有工作階段在線上，核准暫時改在終端機進行',
    allowed: 'symphony: 已在網頁上允許',
    denied: 'symphony: 已在網頁上拒絕',
  },
  ja: {
    unreachable: 'symphony: bridge（{bridge}）に接続できません。承認はひとまずターミナルで行います',
    started: 'symphony: bridge（{bridge}）をバックグラウンドで起動しました。オフィスを開くには symphony を実行してください',
    badToken: 'symphony: bridge がこの SYMPHONY_TOKEN を認識しません。承認はターミナルで行います',
    notBridge: 'symphony: {bridge} で応答しているのは、このセッションが信頼できる symphony の bridge ではないか、このプラグインより古いものです（symphony を更新してください）。承認はターミナルで行います',
    insecure: 'symphony: SYMPHONY_BRIDGE（{bridge}）はこのマシン上にあるか、https を使う必要があります。承認はターミナルで行います',
    busy: 'symphony: このアカウントには既にオンラインのセッションがあります。承認はひとまずターミナルで行います',
    allowed: 'symphony: Web ページで許可されました',
    denied: 'symphony: Web ページで拒否されました',
  },
  ko: {
    unreachable: 'symphony: bridge({bridge})에 연결할 수 없습니다. 승인은 당분간 터미널에서 처리합니다',
    started: 'symphony: 백그라운드에서 bridge({bridge})를 시작했습니다. 사무실을 열려면 symphony를 실행하세요',
    badToken: 'symphony: bridge가 이 SYMPHONY_TOKEN을 알지 못합니다. 승인은 터미널에서 처리합니다',
    notBridge: 'symphony: {bridge}에서 응답하는 것은 이 세션이 신뢰할 수 있는 symphony bridge가 아니거나 이 플러그인보다 오래된 버전입니다(symphony를 업데이트하세요). 승인은 터미널에서 처리합니다',
    insecure: 'symphony: SYMPHONY_BRIDGE({bridge})는 이 컴퓨터에 있거나 https를 사용해야 합니다. 승인은 터미널에서 처리합니다',
    busy: 'symphony: 이 계정에는 이미 온라인 세션이 있습니다. 승인은 당분간 터미널에서 처리합니다',
    allowed: 'symphony: 웹 페이지에서 허용됨',
    denied: 'symphony: 웹 페이지에서 거부됨',
  },
  es: {
    unreachable: 'symphony: no se puede conectar con el bridge ({bridge}); por ahora las aprobaciones se hacen en la terminal',
    started: 'symphony: se inició el bridge ({bridge}) en segundo plano; ejecuta symphony para abrir la oficina',
    badToken: 'symphony: el bridge no reconoce este SYMPHONY_TOKEN; las aprobaciones se hacen en la terminal',
    notBridge: 'symphony: lo que responde en {bridge} no es un bridge de symphony en el que esta sesión pueda confiar, o es más antiguo que este plugin (actualiza symphony); las aprobaciones se hacen en la terminal',
    insecure: 'symphony: SYMPHONY_BRIDGE ({bridge}) debe estar en esta máquina o usar https; las aprobaciones se hacen en la terminal',
    busy: 'symphony: esta cuenta ya tiene una sesión en línea; por ahora las aprobaciones se hacen en la terminal',
    allowed: 'symphony: permitido en la página web',
    denied: 'symphony: denegado en la página web',
  },
  fr: {
    unreachable: 'symphony : impossible de joindre le bridge ({bridge}) ; les approbations se font dans le terminal pour l’instant',
    started: 'symphony : bridge ({bridge}) démarré en arrière-plan ; lancez symphony pour ouvrir le bureau',
    badToken: 'symphony : le bridge ne reconnaît pas ce SYMPHONY_TOKEN ; les approbations se font dans le terminal',
    notBridge: 'symphony : ce qui répond à {bridge} n’est pas un bridge symphony auquel cette session peut se fier, ou il est plus ancien que ce plugin (mettez symphony à jour) ; les approbations se font dans le terminal',
    insecure: 'symphony : SYMPHONY_BRIDGE ({bridge}) doit être sur cette machine ou utiliser https ; les approbations se font dans le terminal',
    busy: 'symphony : ce compte a déjà une session en ligne ; les approbations se font dans le terminal pour l’instant',
    allowed: 'symphony : autorisé dans la page web',
    denied: 'symphony : refusé dans la page web',
  },
  de: {
    unreachable: 'symphony: Bridge ({bridge}) nicht erreichbar; Freigaben laufen vorerst im Terminal',
    started: 'symphony: Bridge ({bridge}) im Hintergrund gestartet; starte symphony, um das Büro zu öffnen',
    badToken: 'symphony: Die Bridge kennt dieses SYMPHONY_TOKEN nicht; Freigaben laufen im Terminal',
    notBridge: 'symphony: Was unter {bridge} antwortet, ist keine Bridge von symphony, der diese Sitzung vertrauen kann, oder sie ist älter als dieses Plugin (symphony aktualisieren); Freigaben laufen im Terminal',
    insecure: 'symphony: SYMPHONY_BRIDGE ({bridge}) muss auf diesem Rechner liegen oder https verwenden; Freigaben laufen im Terminal',
    busy: 'symphony: Dieses Konto hat bereits eine Sitzung online; Freigaben laufen vorerst im Terminal',
    allowed: 'symphony: auf der Webseite erlaubt',
    denied: 'symphony: auf der Webseite abgelehnt',
  },
  'pt-BR': {
    unreachable: 'symphony: não foi possível conectar ao bridge ({bridge}); por enquanto as aprovações ficam no terminal',
    started: 'symphony: bridge ({bridge}) iniciado em segundo plano; execute symphony para abrir o escritório',
    badToken: 'symphony: o bridge não reconhece este SYMPHONY_TOKEN; as aprovações ficam no terminal',
    notBridge: 'symphony: o que responde em {bridge} não é um bridge do symphony em que esta sessão possa confiar, ou é mais antigo que este plugin (atualize o symphony); as aprovações ficam no terminal',
    insecure: 'symphony: SYMPHONY_BRIDGE ({bridge}) precisa estar nesta máquina ou usar https; as aprovações ficam no terminal',
    busy: 'symphony: esta conta já tem uma sessão online; por enquanto as aprovações ficam no terminal',
    allowed: 'symphony: permitido na página web',
    denied: 'symphony: negado na página web',
  },
  ru: {
    unreachable: 'symphony: не удаётся подключиться к bridge ({bridge}); пока подтверждения — в терминале',
    started: 'symphony: bridge ({bridge}) запущен в фоне; чтобы открыть офис, запустите symphony',
    badToken: 'symphony: bridge не знает этот SYMPHONY_TOKEN; подтверждения — в терминале',
    notBridge: 'symphony: по адресу {bridge} отвечает не bridge symphony, которому этот сеанс может доверять, или он старше этого плагина (обновите symphony); подтверждения — в терминале',
    insecure: 'symphony: SYMPHONY_BRIDGE ({bridge}) должен быть на этом компьютере или использовать https; подтверждения — в терминале',
    busy: 'symphony: у этой учётной записи уже есть сеанс онлайн; пока подтверждения — в терминале',
    allowed: 'symphony: разрешено на веб-странице',
    denied: 'symphony: отклонено на веб-странице',
  },
} as const

export type Lang = keyof typeof MESSAGES
export type MessageKey = keyof (typeof MESSAGES)['en']
export const LANGS = Object.keys(MESSAGES) as Lang[]

// matchLang maps a language tag ("zh_CN.UTF-8", "zh-Hant-HK", "pt", "en-US") to a supported language.
export function matchLang(tag: string | undefined): Lang | undefined {
  const parts = (tag ?? '').trim().split(/[.@]/)[0].replace(/_/g, '-').toLowerCase().split('-')
  if (parts[0] === 'zh') return parts.slice(1).some(p => ['hant', 'tw', 'hk', 'mo'].includes(p)) ? 'zh-TW' : 'zh-CN'
  if (parts[0] === 'pt') return 'pt-BR'
  return LANGS.find(l => l.toLowerCase() === parts[0])
}

// message returns the message for key in lang, with each {name} filled in from params.
export function message(lang: Lang, key: MessageKey, params: Record<string, string> = {}): string {
  const text: string = MESSAGES[lang][key]
  return text.replace(/\{(\w+)\}/g, (m, name: string) => params[name] ?? m)
}
