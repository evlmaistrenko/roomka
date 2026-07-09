// PeerMedia turns a remote peer's decoded video frames back into a MediaStream
// (via a video MediaStreamTrackGenerator) so a <video> element renders them
// natively: correct aspect ratio and smooth playback. Audio is played
// separately through Web Audio (see AudioPlayer) rather than an audio track
// generator, which is proprietary and unreliable for element playback.
export class PeerMedia {
	readonly stream = new MediaStream()
	private readonly videoWriter: WritableStreamDefaultWriter<VideoFrame>
	private videoBusy = false

	constructor() {
		const video = new MediaStreamTrackGenerator<VideoFrame>({ kind: "video" })
		this.stream.addTrack(video)
		this.videoWriter = video.writable.getWriter()
	}

	writeVideo(frame: VideoFrame) {
		// Drop rather than queue when the sink is backpressured, to bound memory.
		if (this.videoBusy) {
			frame.close()
			return
		}
		this.videoBusy = true
		void this.videoWriter
			.write(frame)
			// A successful write hands the frame to the track sink (which owns and
			// closes it); a rejected write (writer/track closed) does not, so reclaim
			// it here to avoid leaking the frame and swallow the rejection.
			.catch(() => frame.close())
			.finally(() => {
				this.videoBusy = false
			})
	}

	close() {
		void this.videoWriter.close().catch(() => {})
		for (const track of this.stream.getTracks()) track.stop()
	}
}
