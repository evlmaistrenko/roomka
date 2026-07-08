// MediaStreamTrackProcessor (capture: track -> VideoFrame/AudioData) and
// MediaStreamTrackGenerator (playback: VideoFrame/AudioData -> track) are the
// Chromium "breakout box" APIs. They're not in TypeScript's lib.dom, and the
// generator (especially for audio) is Chrome-proprietary — fine here, since the
// whole pipeline (WebTransport + WebCodecs) is Chromium-only.
export {}

declare global {
  interface MediaStreamTrackProcessorInit {
    track: MediaStreamTrack
    maxBufferSize?: number
  }

  class MediaStreamTrackProcessor<T = VideoFrame> {
    constructor(init: MediaStreamTrackProcessorInit)
    readonly readable: ReadableStream<T>
  }

  interface MediaStreamTrackGeneratorInit {
    kind: 'audio' | 'video'
  }

  class MediaStreamTrackGenerator<
    T extends VideoFrame | AudioData = VideoFrame,
  > extends MediaStreamTrack {
    constructor(init: MediaStreamTrackGeneratorInit)
    readonly writable: WritableStream<T>
  }
}
