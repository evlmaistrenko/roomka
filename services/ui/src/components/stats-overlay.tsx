import { useEffect, useRef, useState } from "react"

import { type BroadcastConfig } from "@/lib/broadcast-config"
import { type ReceiverStats } from "@/lib/receiver"

// A small "stats for nerds" overlay for the active tile. For a remote peer it
// shows live receive stats (polled once a second, with bitrate/fps derived from
// deltas); for the local tile it shows the send parameters from the config.
export function StatsOverlay({
	active,
	getStats,
	config,
}: {
	active: number | "local"
	getStats: () => ReceiverStats[]
	config: BroadcastConfig
}) {
	const [rows, setRows] = useState<[string, string][]>([])
	const prev = useRef<{ bytes: number; frames: number; t: number } | null>(null)

	useEffect(() => {
		prev.current = null

		const tick = () => {
			if (active === "local") {
				setRows([
					["stream", "local (sending)"],
					["codec", config.codec],
					["resolution", `${config.height}p`],
					["framerate", `${config.framerate} fps`],
					["bitrate", `${(config.bitrate / 1_000_000).toFixed(1)} Mbps`],
					["bitrate mode", config.bitrateMode],
					["keyframe", `${config.keyframeIntervalMs} ms`],
					["latency", config.latencyMode],
					["hardware", config.hardwareAcceleration],
					["datagram", config.datagramSize],
				])
				return
			}

			const s = getStats().find((row) => row.senderId === active)
			if (!s) {
				setRows([
					["stream", `peer ${active.toString(16)}`],
					["status", "waiting…"],
				])
				return
			}

			const now = performance.now()
			let bitrateMbps = 0
			let fps = 0
			if (prev.current) {
				const dt = (now - prev.current.t) / 1000
				if (dt > 0) {
					bitrateMbps = ((s.bytes - prev.current.bytes) * 8) / dt / 1_000_000
					fps = (s.framesDecoded - prev.current.frames) / dt
				}
			}
			prev.current = { bytes: s.bytes, frames: s.framesDecoded, t: now }

			setRows([
				["stream", `peer ${active.toString(16)}`],
				["codec", s.codec || "—"],
				["resolution", s.width ? `${s.width}×${s.height}` : "—"],
				["bitrate in", `${bitrateMbps.toFixed(2)} Mbps`],
				["fps", fps.toFixed(0)],
				["frames", `${s.framesDecoded} dec / ${s.framesDropped} drop`],
				["decode queue", String(s.decodeQueueSize)],
				["datagrams", String(s.datagrams)],
				["decrypt fails", String(s.decryptFailures)],
				["decoder errs", String(s.decoderErrors)],
				[
					"audio",
					s.audioSampleRate
						? `${(s.audioSampleRate / 1000).toFixed(1)} kHz ×${s.audioChannels}`
						: "—",
				],
			])
		}

		// setTimeout(0) so the first paint isn't synchronous setState-in-effect, and
		// isn't blank for a whole second.
		const initial = setTimeout(tick, 0)
		const timer = setInterval(tick, 1000)
		return () => {
			clearTimeout(initial)
			clearInterval(timer)
		}
	}, [active, getStats, config])

	return (
		<div className="pointer-events-none absolute left-3 top-3 z-10 rounded-md bg-black/75 p-2 font-mono text-[11px] leading-tight text-white shadow-lg">
			<table>
				<tbody>
					{rows.map(([key, value]) => (
						<tr key={key}>
							<td className="pr-3 text-white/60">{key}</td>
							<td className="text-right tabular-nums">{value}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	)
}
