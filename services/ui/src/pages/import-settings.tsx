import { KeyRound, Upload } from "lucide-react"
import { useRef, useState } from "react"
import { useNavigate } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { resetKey } from "@/lib/e2ee"
import { loadSettings, parseSettingsFile, saveSettings } from "@/lib/settings"
import { cn } from "@/lib/utils"

// Shown on startup (and reachable from Settings) to import the JSON settings
// file. It must contain the end-to-end encryption key; an invalid key is
// rejected here with an explicit message instead of silently failing later.
export function ImportSettings() {
	const navigate = useNavigate()
	const inputRef = useRef<HTMLInputElement>(null)
	const [error, setError] = useState<string | null>(null)
	const [busy, setBusy] = useState(false)
	const [dragging, setDragging] = useState(false)

	const hasSettings = loadSettings() !== null

	const handleFile = async (file: File) => {
		setError(null)
		setBusy(true)
		try {
			const settings = await parseSettingsFile(await file.text())
			saveSettings(settings)
			resetKey() // drop any cached key so the new one takes effect
			navigate("/", { replace: true })
		} catch (err) {
			setError(err instanceof Error ? err.message : String(err))
			setBusy(false)
		}
	}

	return (
		<div className="flex min-h-dvh items-center justify-center p-4">
			<div className="w-full max-w-md rounded-lg border bg-card p-6 text-card-foreground shadow-sm">
				<div className="mb-2 flex items-center gap-2">
					<KeyRound className="size-5" />
					<h1 className="text-lg font-semibold">Import settings</h1>
				</div>
				<p className="mb-4 text-sm text-muted-foreground">
					Load your settings file to join. It must include your end-to-end
					encryption key — you need the same key as everyone else in the room.
				</p>

				<div
					onDragOver={(event) => {
						event.preventDefault()
						setDragging(true)
					}}
					onDragLeave={() => setDragging(false)}
					onDrop={(event) => {
						event.preventDefault()
						setDragging(false)
						const file = event.dataTransfer.files?.[0]
						if (file) void handleFile(file)
					}}
					className={cn(
						"flex flex-col items-center gap-3 rounded-lg border border-dashed p-6 text-center transition",
						dragging ? "border-primary bg-primary/5" : "border-input",
					)}
				>
					<Upload className="size-8 text-muted-foreground" />
					<p className="text-sm text-muted-foreground">
						Drop a <code className="font-mono">.json</code> file here, or
					</p>
					<input
						ref={inputRef}
						type="file"
						accept="application/json,.json"
						className="hidden"
						onChange={(event) => {
							const file = event.target.files?.[0]
							event.target.value = "" // allow re-selecting the same file
							if (file) void handleFile(file)
						}}
					/>
					<Button
						disabled={busy}
						onClick={() => inputRef.current?.click()}
					>
						Choose file
					</Button>
				</div>

				{error && (
					<p className="mt-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
						{error}
					</p>
				)}

				{hasSettings && (
					<button
						type="button"
						onClick={() => navigate("/", { replace: true })}
						className="mt-4 w-full cursor-pointer text-center text-sm text-muted-foreground underline-offset-4 hover:underline"
					>
						Keep current settings
					</button>
				)}
			</div>
		</div>
	)
}
