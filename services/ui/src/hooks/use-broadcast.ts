import { useCallback, useEffect, useRef, useState } from "react"

import { AudioPlayer } from "@/lib/audio-player"
import {
	type BroadcastConfig,
	DATAGRAM_SIZE_CAPS,
	resolveDatagramSize,
} from "@/lib/broadcast-config"
import { PeerMedia } from "@/lib/peer-media"
import { type ReceiverStats, startReceiving } from "@/lib/receiver"
import { type ScreenShare, startShare } from "@/lib/sender"
import { connect } from "@/lib/transport"

const SENDER_TIMEOUT_MS = 3000

function randomId(): number {
	return Math.floor(Math.random() * 0xffffffff)
}

export function useBroadcast() {
	const [senders, setSenders] = useState<number[]>([])
	const [isSharing, setIsSharing] = useState(false)
	const [localStream, setLocalStream] = useState<MediaStream | null>(null)
	const [error, setError] = useState<string | null>(null)

	const transportRef = useRef<WebTransport | null>(null)
	const writerRef = useRef<WritableStreamDefaultWriter | null>(null)
	const senderIdRef = useRef<number>(randomId())
	const shareRef = useRef<ScreenShare | null>(null)
	const peers = useRef(new Map<number, PeerMedia>())
	const audioPlayers = useRef(new Map<number, AudioPlayer>())
	const playingPeers = useRef(new Set<number>())
	const lastSeen = useRef(new Map<number, number>())
	const volumeRef = useRef(1)
	const getStatsRef = useRef<(() => ReceiverStats[]) | null>(null)

	// Ensure a PeerMedia (and thus its MediaStream) exists as soon as a sender is
	// seen — from audio OR video — so the tile can bind srcObject even when the
	// first reassembled packet is audio (which is common: small Opus packets
	// reassemble before a multi-fragment video keyframe).
	const ensurePeer = useCallback((senderId: number): PeerMedia => {
		lastSeen.current.set(senderId, performance.now())
		let peer = peers.current.get(senderId)
		if (!peer) {
			peer = new PeerMedia()
			peers.current.set(senderId, peer)
			setSenders((prev) =>
				prev.includes(senderId) ? prev : [...prev, senderId],
			)
		}
		return peer
	}, [])

	const onVideoFrame = useCallback(
		(senderId: number, frame: VideoFrame) =>
			ensurePeer(senderId).writeVideo(frame),
		[ensurePeer],
	)

	const onDecryptFailure = useCallback(() => {
		setError(
			"Decryption keeps failing — check that your E2EE key matches the room’s.",
		)
	}, [])

	const onAudioData = useCallback(
		(senderId: number, data: AudioData) => {
			ensurePeer(senderId)
			let player = audioPlayers.current.get(senderId)
			if (!player) {
				player = new AudioPlayer()
				player.setVolume(volumeRef.current)
				if (playingPeers.current.has(senderId)) player.resume()
				audioPlayers.current.set(senderId, player)
			}
			player.play(data)
		},
		[ensurePeer],
	)

	useEffect(() => {
		let cancelled = false
		let stopReceiving: (() => void) | null = null
		const activePeers = peers.current
		const activePlayers = audioPlayers.current
		const activePlaying = playingPeers.current

		connect()
			.then((transport) => {
				if (cancelled) {
					transport.close()
					return
				}
				transportRef.current = transport
				writerRef.current = transport.datagrams.writable.getWriter()
				const receiver = startReceiving(
					transport,
					onVideoFrame,
					onAudioData,
					onDecryptFailure,
				)
				stopReceiving = receiver.stop
				getStatsRef.current = receiver.getStats
			})
			.catch((e: unknown) => {
				if (!cancelled) setError(String(e))
			})

		const prune = setInterval(() => {
			const now = performance.now()
			const dead: number[] = []
			for (const [id, seen] of lastSeen.current) {
				if (now - seen >= SENDER_TIMEOUT_MS) dead.push(id)
			}
			if (dead.length === 0) return
			for (const id of dead) {
				activePeers.get(id)?.close()
				activePeers.delete(id)
				activePlayers.get(id)?.close()
				activePlayers.delete(id)
				activePlaying.delete(id)
				lastSeen.current.delete(id)
			}
			setSenders((prev) => prev.filter((id) => !dead.includes(id)))
		}, 1000)

		return () => {
			cancelled = true
			clearInterval(prune)
			stopReceiving?.()
			getStatsRef.current = null
			shareRef.current?.stop()
			shareRef.current = null
			writerRef.current?.releaseLock()
			transportRef.current?.close()
			transportRef.current = null
			writerRef.current = null
			for (const peer of activePeers.values()) peer.close()
			activePeers.clear()
			for (const player of activePlayers.values()) player.close()
			activePlayers.clear()
			activePlaying.clear()
		}
	}, [onVideoFrame, onAudioData, onDecryptFailure])

	// Send one keyframe over a fresh reliable unidirectional stream (stream mode).
	// Best-effort like the datagram writer: if the transport is closing, opening or
	// writing rejects, the keyframe is dropped, and the next one recovers.
	const sendKeyframe = useCallback(async (message: Uint8Array) => {
		const transport = transportRef.current
		if (!transport) return
		let writable: WritableStream
		try {
			writable = await transport.createUnidirectionalStream()
		} catch {
			return
		}
		const writer = writable.getWriter()
		try {
			await writer.write(message)
			await writer.close()
		} catch {
			try {
				await writer.abort()
			} catch {
				// stream already errored/closed — nothing to reclaim
			}
		}
	}, [])

	const startSharing = useCallback(
		async (config: BroadcastConfig) => {
			const writer = writerRef.current
			const transport = transportRef.current
			if (!writer || !transport || isSharing) return
			try {
				if (!transport.datagrams.maxDatagramSize) {
					setError("This connection does not support datagrams.")
					return
				}
				const share = await startShare(
					// Datagram delivery is best-effort; a write rejects when the transport
					// closes (e.g. on stop/unmount). Swallow it so a closing transport
					// doesn't spray unhandled rejections — real failures surface elsewhere.
					(datagram) => void writer.write(datagram).catch(() => {}),
					sendKeyframe,
					senderIdRef.current,
					// Resolve the chosen datagram-size mode against the live path MTU, read
					// fresh each frame so a mid-session MTU drop adapts. The broadcast server
					// fans one datagram out to all viewers and can't re-fragment, so the fixed
					// modes stay below the size every viewer's path is guaranteed to accept.
					() =>
						resolveDatagramSize(
							config.datagramSize,
							transport.datagrams.maxDatagramSize || DATAGRAM_SIZE_CAPS.safe,
						),
					config,
				)
				shareRef.current = share
				setLocalStream(share.stream)
				setIsSharing(true)
			} catch (e: unknown) {
				setError(String(e))
			}
		},
		[isSharing, sendKeyframe],
	)

	const stopSharing = useCallback(() => {
		shareRef.current?.stop()
		shareRef.current = null
		setLocalStream(null)
		setIsSharing(false)
	}, [])

	// Start/stop a remote peer's audio. Called from the tile's Play button, whose
	// click is the user gesture that lets the AudioContext resume (autoplay policy).
	const setPeerPlaying = useCallback((senderId: number, playing: boolean) => {
		if (playing) {
			playingPeers.current.add(senderId)
			audioPlayers.current.get(senderId)?.resume()
		} else {
			playingPeers.current.delete(senderId)
			audioPlayers.current.get(senderId)?.suspend()
		}
	}, [])

	const getStream = useCallback(
		(senderId: number): MediaStream | null =>
			peers.current.get(senderId)?.stream ?? null,
		[],
	)

	// Snapshot of per-sender receive stats for the debug overlay (empty until
	// connected). Stable identity — the overlay polls it on its own interval.
	const getStats = useCallback(
		(): ReceiverStats[] => getStatsRef.current?.() ?? [],
		[],
	)

	// Master playback volume (0..1) applied to every peer's audio. Only the active
	// peer is unmuted at a time, so one control governs what the viewer hears.
	const setVolume = useCallback((volume: number) => {
		volumeRef.current = volume
		for (const player of audioPlayers.current.values()) player.setVolume(volume)
	}, [])

	return {
		senders,
		isSharing,
		localStream,
		error,
		startSharing,
		stopSharing,
		setPeerPlaying,
		setVolume,
		getStream,
		getStats,
	}
}
