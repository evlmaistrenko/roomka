// The sharer's video encode + transport settings, chosen in Settings and
// persisted to localStorage. This supersedes the fixed VideoPreset list for the
// UI (presets remain in video-presets.ts as reference data); every field here is
// exposed individually so real-network behavior can be tuned and tested.
import { SAFE_MAX_DATAGRAM_SIZE } from './config'

export type CodecFamily = 'h264' | 'vp8' | 'vp9'

// Datagram size the sharer targets. Fixed caps are clamped to the sharer's own
// live path MTU; 'max' uses that live size uncapped. The relay forwards one
// datagram to every viewer without re-fragmenting, so anything above ~1150 (the
// guaranteed QUIC floor) only works if every viewer's path is wide enough too —
// which is exactly what the larger options let you test.
export type DatagramSizeMode = 'small' | 'safe' | 'large' | 'max'

export type BroadcastConfig = {
  codec: CodecFamily
  height: number
  framerate: number
  bitrate: number
  latencyMode: 'quality' | 'realtime'
  hardwareAcceleration: 'no-preference' | 'prefer-hardware' | 'prefer-software'
  keyframeIntervalMs: number
  bitrateMode: 'constant' | 'variable'
  datagramSize: DatagramSizeMode
}

// --- Option lists for the Settings UI (extend freely) ---

export const CODEC_OPTIONS: { value: CodecFamily; label: string }[] = [
  { value: 'h264', label: 'H.264' },
  { value: 'vp8', label: 'VP8' },
  { value: 'vp9', label: 'VP9' },
]
export const HEIGHT_OPTIONS = [480, 720, 1080, 1440]
export const FRAMERATE_OPTIONS = [15, 24, 30, 60]
export const BITRATE_OPTIONS = [
  500_000, 1_000_000, 2_500_000, 4_000_000, 6_000_000, 8_000_000, 12_000_000,
]
export const LATENCY_OPTIONS: BroadcastConfig['latencyMode'][] = [
  'realtime',
  'quality',
]
export const HARDWARE_OPTIONS: BroadcastConfig['hardwareAcceleration'][] = [
  'no-preference',
  'prefer-hardware',
  'prefer-software',
]
export const KEYFRAME_INTERVAL_OPTIONS = [500, 1000, 2000, 4000]
export const BITRATE_MODE_OPTIONS: BroadcastConfig['bitrateMode'][] = [
  'variable',
  'constant',
]

// Fixed datagram caps per mode; 'max' means "use the live path size uncapped".
export const DATAGRAM_SIZE_CAPS: Record<
  Exclude<DatagramSizeMode, 'max'>,
  number
> = {
  small: 600,
  safe: SAFE_MAX_DATAGRAM_SIZE,
  large: 1350,
}
export const DATAGRAM_SIZE_OPTIONS: { value: DatagramSizeMode; label: string }[] =
  [
    { value: 'small', label: `Small (${DATAGRAM_SIZE_CAPS.small})` },
    { value: 'safe', label: `Safe (${DATAGRAM_SIZE_CAPS.safe})` },
    { value: 'large', label: `Large (${DATAGRAM_SIZE_CAPS.large})` },
    { value: 'max', label: 'Max (live path)' },
  ]

export const DEFAULT_BROADCAST_CONFIG: BroadcastConfig = {
  codec: 'vp8',
  height: 1080,
  framerate: 60,
  bitrate: 8_000_000,
  latencyMode: 'realtime',
  hardwareAcceleration: 'no-preference',
  keyframeIntervalMs: 2000,
  bitrateMode: 'variable',
  datagramSize: 'safe',
}

// Resolve a datagram-size mode against the connection's current maxDatagramSize.
export function resolveDatagramSize(
  mode: DatagramSizeMode,
  liveMax: number,
): number {
  if (mode === 'max') return liveMax
  return Math.min(DATAGRAM_SIZE_CAPS[mode], liveMax)
}

// --- Codec string construction ---
// The codec string encodes profile + level, and the level must be high enough
// for the resolution/framerate (an under-declared H.264/VP9 level is rejected).
// We derive the smallest sufficient level from the frame size and fps rather
// than hardcoding a table per resolution.

// H.264 levels: [level byte, max frame size (macroblocks), max macroblocks/sec].
const H264_LEVELS: [number, number, number][] = [
  [0x1e, 1620, 40500], // 3.0
  [0x1f, 3600, 108000], // 3.1
  [0x20, 5120, 216000], // 3.2
  [0x28, 8192, 245760], // 4.0
  [0x2a, 8704, 522240], // 4.2
  [0x32, 22080, 589824], // 5.0
  [0x33, 36864, 983040], // 5.1
  [0x34, 36864, 2073600], // 5.2
]

// VP9 levels: [level code (as in the codec string), max luma samples/sec, max
// luma picture size]. The picture-size cap binds before the sample-rate cap at
// low framerates, so both must be checked to avoid under-declaring the level.
const VP9_LEVELS: [number, number, number][] = [
  [30, 20736000, 552960], // 3.0
  [31, 36864000, 983040], // 3.1
  [40, 83558400, 2228224], // 4.0
  [41, 160432128, 4194304], // 4.1
  [50, 311951360, 8912896], // 5.0
  [51, 588251136, 8912896], // 5.1
  [52, 1176502272, 8912896], // 5.2
]

export function buildCodecString(
  codec: CodecFamily,
  width: number,
  height: number,
  framerate: number,
): string {
  if (codec === 'vp8') return 'vp8'

  if (codec === 'vp9') {
    const picture = width * height
    const luma = picture * framerate
    const level =
      VP9_LEVELS.find(
        ([, maxLuma, maxPicture]) => luma <= maxLuma && picture <= maxPicture,
      )?.[0] ?? VP9_LEVELS[VP9_LEVELS.length - 1][0]
    return `vp09.00.${String(level).padStart(2, '0')}.08`
  }

  // h264: High profile (0x64), no constraint flags (0x00), derived level.
  const frameSize = Math.ceil(width / 16) * Math.ceil(height / 16)
  const mbPerSec = frameSize * framerate
  const level =
    H264_LEVELS.find(
      ([, maxFS, maxMBPS]) => frameSize <= maxFS && mbPerSec <= maxMBPS,
    )?.[0] ?? H264_LEVELS[H264_LEVELS.length - 1][0]
  return `avc1.6400${level.toString(16).padStart(2, '0')}`
}

// Build the WebCodecs encoder config for a concrete frame size. H.264 is emitted
// as Annex-B so SPS/PPS are inlined with every keyframe — over a lossy datagram
// relay a late-joining or packet-dropping receiver recovers at the next keyframe
// without an out-of-band decoder description.
export function buildEncoderConfig(
  config: BroadcastConfig,
  width: number,
  height: number,
): VideoEncoderConfig {
  const encoderConfig: VideoEncoderConfig = {
    codec: buildCodecString(config.codec, width, height, config.framerate),
    width,
    height,
    bitrate: config.bitrate,
    framerate: config.framerate,
    latencyMode: config.latencyMode,
    hardwareAcceleration: config.hardwareAcceleration,
    bitrateMode: config.bitrateMode,
  }
  if (config.codec === 'h264') encoderConfig.avc = { format: 'annexb' }
  return encoderConfig
}

// Probe whether the device can encode the chosen config (16:9 assumed for the
// probe), so Settings can flag an unsupported combination.
export async function isConfigSupported(
  config: BroadcastConfig,
): Promise<boolean> {
  try {
    const width = Math.max(2, Math.round((config.height * 16) / 9) & ~1)
    const { supported } = await VideoEncoder.isConfigSupported(
      buildEncoderConfig(config, width, config.height),
    )
    return supported === true
  } catch {
    return false
  }
}

// --- Persistence ---

const STORAGE_KEY = 'roomka:broadcast-config'

function coerce<T>(value: unknown, allowed: readonly T[], fallback: T): T {
  return allowed.includes(value as T) ? (value as T) : fallback
}

export function loadBroadcastConfig(): BroadcastConfig {
  let parsed: Partial<BroadcastConfig> = {}
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) parsed = JSON.parse(raw) as Partial<BroadcastConfig>
  } catch {
    // corrupt entry — every field falls back to its default below
  }
  // Validate each field against its option list (not just fill missing keys): a
  // tampered or schema-drifted value (e.g. datagramSize:'xyz' → NaN datagram
  // size → zero datagrams sent) must never reach the encoder or fragmenter.
  const d = DEFAULT_BROADCAST_CONFIG
  return {
    codec: coerce(
      parsed.codec,
      CODEC_OPTIONS.map((option) => option.value),
      d.codec,
    ),
    height: coerce(parsed.height, HEIGHT_OPTIONS, d.height),
    framerate: coerce(parsed.framerate, FRAMERATE_OPTIONS, d.framerate),
    bitrate: coerce(parsed.bitrate, BITRATE_OPTIONS, d.bitrate),
    latencyMode: coerce(parsed.latencyMode, LATENCY_OPTIONS, d.latencyMode),
    hardwareAcceleration: coerce(
      parsed.hardwareAcceleration,
      HARDWARE_OPTIONS,
      d.hardwareAcceleration,
    ),
    keyframeIntervalMs: coerce(
      parsed.keyframeIntervalMs,
      KEYFRAME_INTERVAL_OPTIONS,
      d.keyframeIntervalMs,
    ),
    bitrateMode: coerce(parsed.bitrateMode, BITRATE_MODE_OPTIONS, d.bitrateMode),
    datagramSize: coerce(
      parsed.datagramSize,
      DATAGRAM_SIZE_OPTIONS.map((option) => option.value),
      d.datagramSize,
    ),
  }
}

export function saveBroadcastConfig(config: BroadcastConfig) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(config))
}
