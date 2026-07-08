// Broadcast quality presets the sharer picks from in Settings. A preset is
// purely a *sender-side* encode choice: codec + capture height + framerate +
// bitrate. Receivers never pick a preset — they learn each sender's codec from
// the stream itself (keyframes are tagged with the codec string), and derive
// frame dimensions from the decoded bitstream. Only the height is constrained on
// capture so the source's aspect ratio is preserved (see sender.ts).

export type VideoPreset = {
  id: string
  family: string // grouping label in the Settings UI
  label: string // e.g. "1080p60"; where fps is omitted the framerate is 30
  codec: string // WebCodecs codec string passed to VideoEncoder.configure
  height: number
  framerate: number
  bitrate: number
}

// H.264 uses High profile with a level matching the resolution/fps; the level is
// the last byte of the codec string (0x2A=4.2, 0x28=4.0, 0x20=3.2, 0x1F=3.1,
// 0x1E=3.0). H.264 is widely hardware-accelerated. VP8 is universally supported
// but software-encoded (heavier CPU at high res). VP9 compresses better than VP8
// at similar cost on machines with hardware VP9 encode.
export const VIDEO_PRESETS: VideoPreset[] = [
  { id: 'h264-1080p60', family: 'H.264', label: '1080p60', codec: 'avc1.64002A', height: 1080, framerate: 60, bitrate: 8_000_000 },
  { id: 'h264-1080p', family: 'H.264', label: '1080p', codec: 'avc1.640028', height: 1080, framerate: 30, bitrate: 5_000_000 },
  { id: 'h264-720p60', family: 'H.264', label: '720p60', codec: 'avc1.640020', height: 720, framerate: 60, bitrate: 4_000_000 },
  { id: 'h264-720p', family: 'H.264', label: '720p', codec: 'avc1.64001F', height: 720, framerate: 30, bitrate: 2_500_000 },
  { id: 'h264-480p', family: 'H.264', label: '480p', codec: 'avc1.64001E', height: 480, framerate: 30, bitrate: 1_200_000 },

  { id: 'vp8-1080p60', family: 'VP8', label: '1080p60', codec: 'vp8', height: 1080, framerate: 60, bitrate: 8_000_000 },
  { id: 'vp8-720p', family: 'VP8', label: '720p', codec: 'vp8', height: 720, framerate: 30, bitrate: 2_500_000 },
  { id: 'vp8-480p', family: 'VP8', label: '480p', codec: 'vp8', height: 480, framerate: 30, bitrate: 1_200_000 },

  { id: 'vp9-1080p', family: 'VP9', label: '1080p', codec: 'vp09.00.40.08', height: 1080, framerate: 30, bitrate: 4_000_000 },
  { id: 'vp9-720p', family: 'VP9', label: '720p', codec: 'vp09.00.31.08', height: 720, framerate: 30, bitrate: 2_000_000 },
]

// Default matches the app's previous hardcoded behavior (VP8 1080p60) so nothing
// regresses for users who never open Settings; H.264 is opt-in.
export const DEFAULT_PRESET_ID = 'vp8-1080p60'

export const DEFAULT_PRESET: VideoPreset =
  VIDEO_PRESETS.find((preset) => preset.id === DEFAULT_PRESET_ID) ?? VIDEO_PRESETS[0]

export function presetById(id: string): VideoPreset {
  return VIDEO_PRESETS.find((preset) => preset.id === id) ?? DEFAULT_PRESET
}

// Build the encoder config for a preset at a concrete frame size. H.264 is
// emitted as Annex-B so SPS/PPS are inlined with every keyframe — over a lossy
// datagram relay a late-joining or packet-dropping receiver recovers at the next
// keyframe without needing an out-of-band decoder description.
export function buildEncoderConfig(
  preset: VideoPreset,
  width: number,
  height: number,
): VideoEncoderConfig {
  const config: VideoEncoderConfig = {
    codec: preset.codec,
    width,
    height,
    bitrate: preset.bitrate,
    framerate: preset.framerate,
    latencyMode: 'realtime',
  }
  if (preset.codec.startsWith('avc1')) config.avc = { format: 'annexb' }
  return config
}

// Probe whether the current device can encode this preset, so Settings can
// disable unsupported options (e.g. a Chromium build without H.264 encode).
export async function isPresetSupported(preset: VideoPreset): Promise<boolean> {
  try {
    const width = Math.max(2, Math.round((preset.height * 16) / 9) & ~1)
    const { supported } = await VideoEncoder.isConfigSupported(
      buildEncoderConfig(preset, width, preset.height),
    )
    return supported === true
  } catch {
    return false
  }
}
