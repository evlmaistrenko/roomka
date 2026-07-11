import { KeyRound, X } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"

import { ThemeToggle } from "@/components/theme-toggle"
import { Button } from "@/components/ui/button"
import {
	BITRATE_MODE_OPTIONS,
	BITRATE_OPTIONS,
	type BroadcastConfig,
	CODEC_OPTIONS,
	DATAGRAM_SIZE_OPTIONS,
	FRAMERATE_OPTIONS,
	HARDWARE_OPTIONS,
	HEIGHT_OPTIONS,
	KEYFRAME_INTERVAL_OPTIONS,
	LATENCY_OPTIONS,
	isConfigSupported,
} from "@/lib/broadcast-config"

// A small self-contained modal (no extra dependency) for broadcast settings:
// the sharer's video encode parameters plus the datagram-size mode. Each field
// is exposed individually so behavior can be tuned and tested per network.
export function SettingsDialog({
	open,
	config,
	onChange,
	onClose,
}: {
	open: boolean
	config: BroadcastConfig
	onChange: (config: BroadcastConfig) => void
	onClose: () => void
}) {
	const navigate = useNavigate()
	const dialogRef = useRef<HTMLDivElement>(null)
	// The probed config paired with its result, so the "unsupported" warning only
	// shows once the *current* config's probe resolves — not the previous
	// config's verdict while a new probe is still in flight.
	const [probe, setProbe] = useState<{
		config: BroadcastConfig
		ok: boolean
	} | null>(null)

	useEffect(() => {
		if (!open) return
		let cancelled = false
		void isConfigSupported(config).then((ok) => {
			if (!cancelled) setProbe({ config, ok })
		})
		return () => {
			cancelled = true
		}
	}, [open, config])

	useEffect(() => {
		if (!open) return
		const onKey = (event: KeyboardEvent) => {
			if (event.key === "Escape") onClose()
		}
		document.addEventListener("keydown", onKey)
		return () => document.removeEventListener("keydown", onKey)
	}, [open, onClose])

	// Focus management: move focus into the dialog on open, keep Tab within it,
	// and restore focus to whatever was focused (the trigger) on close.
	useEffect(() => {
		if (!open) return
		const dialog = dialogRef.current
		if (!dialog) return
		const previouslyFocused = document.activeElement as HTMLElement | null

		const focusable = () =>
			Array.from(
				dialog.querySelectorAll<HTMLElement>(
					'button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
				),
			)

		dialog.focus()

		const onKey = (event: KeyboardEvent) => {
			if (event.key !== "Tab") return
			const items = focusable()
			if (items.length === 0) {
				event.preventDefault()
				return
			}
			const first = items[0]
			const last = items[items.length - 1]
			const active = document.activeElement
			const inside = active instanceof HTMLElement && items.includes(active)
			if (event.shiftKey && (!inside || active === first)) {
				event.preventDefault()
				last.focus()
			} else if (!event.shiftKey && (!inside || active === last)) {
				event.preventDefault()
				first.focus()
			}
		}

		document.addEventListener("keydown", onKey)
		return () => {
			document.removeEventListener("keydown", onKey)
			previouslyFocused?.focus?.()
		}
	}, [open])

	if (!open) return null

	return (
		<div
			className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
			onClick={onClose}
		>
			<div
				ref={dialogRef}
				role="dialog"
				aria-modal="true"
				aria-label="Settings"
				tabIndex={-1}
				className="w-full max-w-md rounded-lg border bg-background p-4 shadow-lg outline-none"
				onClick={(event) => event.stopPropagation()}
			>
				<div className="mb-4 flex items-center justify-between">
					<h2 className="text-lg font-semibold">Settings</h2>
					<Button
						variant="ghost"
						size="icon"
						className="size-7"
						onClick={onClose}
						aria-label="Close"
					>
						<X />
					</Button>
				</div>

				<div className="space-y-2">
					<SelectField
						label="Codec"
						value={config.codec}
						options={CODEC_OPTIONS}
						onChange={(codec) => onChange({ ...config, codec })}
					/>
					<SelectField
						label="Resolution"
						value={config.height}
						options={HEIGHT_OPTIONS.map((h) => ({ value: h, label: `${h}p` }))}
						onChange={(height) => onChange({ ...config, height })}
					/>
					<SelectField
						label="Frame rate"
						value={config.framerate}
						options={FRAMERATE_OPTIONS.map((f) => ({
							value: f,
							label: `${f} fps`,
						}))}
						onChange={(framerate) => onChange({ ...config, framerate })}
					/>
					<SelectField
						label="Bitrate"
						value={config.bitrate}
						options={BITRATE_OPTIONS.map((b) => ({
							value: b,
							label: `${b / 1_000_000} Mbps`,
						}))}
						onChange={(bitrate) => onChange({ ...config, bitrate })}
					/>
					<SelectField
						label="Latency mode"
						value={config.latencyMode}
						options={LATENCY_OPTIONS.map((l) => ({ value: l, label: l }))}
						onChange={(latencyMode) => onChange({ ...config, latencyMode })}
					/>
					<SelectField
						label="Hardware"
						value={config.hardwareAcceleration}
						options={HARDWARE_OPTIONS.map((h) => ({ value: h, label: h }))}
						onChange={(hardwareAcceleration) =>
							onChange({ ...config, hardwareAcceleration })
						}
					/>
					<SelectField
						label="Keyframe interval"
						value={config.keyframeIntervalMs}
						options={KEYFRAME_INTERVAL_OPTIONS.map((k) => ({
							value: k,
							label: `${k} ms`,
						}))}
						onChange={(keyframeIntervalMs) =>
							onChange({ ...config, keyframeIntervalMs })
						}
					/>
					<SelectField
						label="Bitrate mode"
						value={config.bitrateMode}
						options={BITRATE_MODE_OPTIONS.map((m) => ({ value: m, label: m }))}
						onChange={(bitrateMode) => onChange({ ...config, bitrateMode })}
					/>
					<SelectField
						label="Max datagram"
						value={config.datagramSize}
						options={DATAGRAM_SIZE_OPTIONS}
						onChange={(datagramSize) => onChange({ ...config, datagramSize })}
					/>
				</div>

				{probe?.config === config && probe.ok === false && (
					<p className="mt-2 text-xs text-destructive">
						This device’s encoder doesn’t support this combination — adjust
						codec, resolution, or frame rate.
					</p>
				)}

				<p className="mt-4 text-xs text-muted-foreground">
					Applies to your next screen share. Receivers detect the codec
					automatically.
				</p>

				<div className="mt-4 flex items-center gap-2 border-t pt-4">
					<p className="text-sm font-medium">Theme</p>
					<ThemeToggle />
				</div>

				<div className="mt-4 border-t pt-4">
					<Button
						variant="outline"
						size="sm"
						onClick={() => {
							onClose()
							navigate("/import")
						}}
					>
						<KeyRound /> Replace settings file
					</Button>
				</div>
			</div>
		</div>
	)
}

// A labeled row with a native <select>. Generic over string/number option
// values; matches the changed option back by its stringified value.
function SelectField<T extends string | number>({
	label,
	value,
	options,
	onChange,
}: {
	label: string
	value: T
	options: { value: T; label: string }[]
	onChange: (value: T) => void
}) {
	return (
		<label className="flex items-center justify-between gap-3 text-sm">
			<span className="text-muted-foreground">{label}</span>
			<select
				value={String(value)}
				onChange={(event) => {
					const chosen = options.find(
						(option) => String(option.value) === event.target.value,
					)
					if (chosen) onChange(chosen.value)
				}}
				className="rounded-md border bg-background px-2 py-1"
			>
				{options.map((option) => (
					<option
						key={String(option.value)}
						value={String(option.value)}
					>
						{option.label}
					</option>
				))}
			</select>
		</label>
	)
}
