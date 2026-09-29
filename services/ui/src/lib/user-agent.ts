export type DeviceKind = "desktop" | "mobile" | "tablet" | "other"

export interface DeviceDescription {
	// "Chrome 153", or the product a non-browser client names itself by
	// ("curl 8.4.0"). Undefined when nothing can be made of it.
	client?: string
	// "Windows", "Android 14", "iOS 17.4".
	system?: string
	kind: DeviceKind
}

// Order matters: most browsers also claim to be the ones they are built on, so
// Edge, Opera and the rest say "Chrome" too, and Chrome says "Safari".
const BROWSERS: [RegExp, string][] = [
	[/\bEdg(?:e|A|iOS)?\/(\d+)/, "Edge"],
	[/\bOPR\/(\d+)/, "Opera"],
	[/\bYaBrowser\/(\d+)/, "Yandex Browser"],
	[/\bSamsungBrowser\/(\d+)/, "Samsung Internet"],
	[/\b(?:Firefox|FxiOS)\/(\d+)/, "Firefox"],
	[/\bHeadlessChrome\/(\d+)/, "Headless Chrome"],
	[/\b(?:Chrome|CriOS)\/(\d+)/, "Chrome"],
	[/\bVersion\/(\d+(?:\.\d+)?).*\bSafari\//, "Safari"],
]

const WINDOWS_VERSIONS: Record<string, string> = {
	"10.0": "",
	"6.3": " 8.1",
	"6.2": " 8",
	"6.1": " 7",
}

// describeUserAgent turns a User-Agent header into what a person recognises
// their device by. It is a best effort over the common clients, not a full
// parser: whatever it cannot place is left out rather than guessed.
export function describeUserAgent(
	userAgent: string | null | undefined,
): DeviceDescription {
	if (!userAgent) return { kind: "other" }
	return {
		client: clientOf(userAgent),
		system: systemOf(userAgent),
		kind: kindOf(userAgent),
	}
}

function clientOf(userAgent: string): string | undefined {
	for (const [pattern, name] of BROWSERS) {
		const version = pattern.exec(userAgent)?.[1]
		if (version) return `${name} ${version}`
	}
	// Not a browser: curl, a script, a library. They lead with their own
	// product token.
	const product = /^([\w.-]+)(?:\/([\w.-]+))?/.exec(userAgent)
	if (product && product[1] !== "Mozilla") {
		return product[2] ? `${product[1]} ${product[2]}` : product[1]
	}
	return undefined
}

function systemOf(userAgent: string): string | undefined {
	const windows = /Windows NT (\d+\.\d+)/.exec(userAgent)
	if (windows) return `Windows${WINDOWS_VERSIONS[windows[1]] ?? ""}`
	const apple = /\b(iPhone|iPad|iPod)\b.*? OS (\d+)(?:_(\d+))?/.exec(userAgent)
	if (apple) {
		const version = apple[3] ? `${apple[2]}.${apple[3]}` : apple[2]
		return `${apple[1] === "iPad" ? "iPadOS" : "iOS"} ${version}`
	}
	const android = /Android (\d+(?:\.\d+)?)/.exec(userAgent)
	if (android) return `Android ${android[1]}`
	if (/\bCrOS\b/.test(userAgent)) return "ChromeOS"
	// Browsers froze the macOS version they report years ago; showing it
	// would only mislead.
	if (/Mac OS X|Macintosh/.test(userAgent)) return "macOS"
	if (/Linux/.test(userAgent)) return "Linux"
	return undefined
}

function kindOf(userAgent: string): DeviceKind {
	if (/\biPad\b|Tablet/.test(userAgent)) return "tablet"
	if (/Android/.test(userAgent)) {
		return /Mobile/.test(userAgent) ? "mobile" : "tablet"
	}
	if (/\biPhone\b|\biPod\b|Mobi/.test(userAgent)) return "mobile"
	if (/Windows NT|Macintosh|Mac OS X|CrOS|X11|Linux/.test(userAgent)) {
		return "desktop"
	}
	return "other"
}
