// Reference broadcast quality presets. These are intentionally kept as data for
// later (a future "quick pick" in Settings): the UI currently exposes every
// encode parameter individually via lib/broadcast-config.ts and does not render
// this list. A preset is purely a *sender-side* encode choice — receivers learn
// each sender's codec from the stream (keyframes are codec-tagged) and derive
// frame dimensions from the decoded bitstream.

export type VideoPreset = {
	id: string
	family: string // grouping label
	label: string // e.g. "1080p60"; where fps is omitted the framerate is 30
	codec: string // full WebCodecs codec string (profile + level)
	height: number
	framerate: number
	bitrate: number
}

// H.264 uses High profile with a level matching the resolution/fps (last byte of
// the codec string). H.264 is widely hardware-accelerated. VP8 is universally
// supported but software-encoded (heavier CPU at high res). VP9 compresses
// better than VP8 at similar cost on machines with hardware VP9 encode.
export const VIDEO_PRESETS: VideoPreset[] = [
	{
		id: "h264-1080p60",
		family: "H.264",
		label: "1080p60",
		codec: "avc1.64002A",
		height: 1080,
		framerate: 60,
		bitrate: 8_000_000,
	},
	{
		id: "h264-1080p",
		family: "H.264",
		label: "1080p",
		codec: "avc1.640028",
		height: 1080,
		framerate: 30,
		bitrate: 5_000_000,
	},
	{
		id: "h264-720p60",
		family: "H.264",
		label: "720p60",
		codec: "avc1.640020",
		height: 720,
		framerate: 60,
		bitrate: 4_000_000,
	},
	{
		id: "h264-720p",
		family: "H.264",
		label: "720p",
		codec: "avc1.64001F",
		height: 720,
		framerate: 30,
		bitrate: 2_500_000,
	},
	{
		id: "h264-480p",
		family: "H.264",
		label: "480p",
		codec: "avc1.64001E",
		height: 480,
		framerate: 30,
		bitrate: 1_200_000,
	},

	{
		id: "vp8-1080p60",
		family: "VP8",
		label: "1080p60",
		codec: "vp8",
		height: 1080,
		framerate: 60,
		bitrate: 8_000_000,
	},
	{
		id: "vp8-720p",
		family: "VP8",
		label: "720p",
		codec: "vp8",
		height: 720,
		framerate: 30,
		bitrate: 2_500_000,
	},
	{
		id: "vp8-480p",
		family: "VP8",
		label: "480p",
		codec: "vp8",
		height: 480,
		framerate: 30,
		bitrate: 1_200_000,
	},

	{
		id: "vp9-1080p",
		family: "VP9",
		label: "1080p",
		codec: "vp09.00.40.08",
		height: 1080,
		framerate: 30,
		bitrate: 4_000_000,
	},
	{
		id: "vp9-720p",
		family: "VP9",
		label: "720p",
		codec: "vp09.00.31.08",
		height: 720,
		framerate: 30,
		bitrate: 2_000_000,
	},
]

export const DEFAULT_PRESET_ID = "vp8-1080p60"

export const DEFAULT_PRESET: VideoPreset =
	VIDEO_PRESETS.find((preset) => preset.id === DEFAULT_PRESET_ID) ??
	VIDEO_PRESETS[0]
