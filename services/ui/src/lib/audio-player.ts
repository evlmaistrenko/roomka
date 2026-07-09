// AudioPlayer schedules decoded AudioData onto the Web Audio clock so packets
// play back gap-free. A small lead absorbs network/decoder jitter. The
// AudioContext starts suspended (autoplay policy); nothing is scheduled until
// resume() is called from a user gesture (the tile's Play button).
export class AudioPlayer {
  private readonly context = new AudioContext()
  // All sources route through a gain node so playback volume is adjustable.
  private readonly gain = this.context.createGain()
  private enabled = false
  private nextStartTime = 0
  private static readonly JITTER_LEAD = 0.06 // seconds
  // Max the schedule may run ahead of the clock before we drop to catch up.
  // Without this, packets arriving slightly faster than realtime would push
  // nextStartTime ever further ahead and latency would grow without bound.
  private static readonly MAX_LEAD = 0.5 // seconds

  constructor() {
    this.gain.connect(this.context.destination)
  }

  play(data: AudioData) {
    // Gate on an explicit flag, not context.state: a fresh AudioContext starts
    // "running" if the tab already has user activation, which would otherwise
    // play audio before the tile's Play button is pressed.
    if (!this.enabled) {
      data.close()
      return
    }

    const now = this.context.currentTime
    // Buffered too far ahead (arriving faster than realtime): drop this packet
    // so latency stays bounded. Already-queued audio keeps playing seamlessly;
    // we just stop extending the schedule until the clock catches up.
    if (this.nextStartTime - now > AudioPlayer.MAX_LEAD) {
      data.close()
      return
    }

    const { numberOfFrames, numberOfChannels, sampleRate } = data
    const buffer = this.context.createBuffer(
      numberOfChannels,
      numberOfFrames,
      sampleRate,
    )
    const channel = new Float32Array(numberOfFrames)
    for (let i = 0; i < numberOfChannels; i++) {
      data.copyTo(channel, { planeIndex: i, format: 'f32-planar' })
      buffer.copyToChannel(channel, i)
    }
    data.close()

    const source = this.context.createBufferSource()
    source.buffer = buffer
    source.connect(this.gain)

    if (this.nextStartTime < now) {
      // Underrun/gap: restart a small lead ahead of the clock.
      this.nextStartTime = now + AudioPlayer.JITTER_LEAD
    }
    source.start(this.nextStartTime)
    this.nextStartTime += buffer.duration
  }

  setVolume(volume: number) {
    this.gain.gain.value = Math.max(0, Math.min(1, volume))
  }

  resume() {
    this.enabled = true
    if (this.context.state === 'suspended') void this.context.resume()
  }

  suspend() {
    this.enabled = false
    if (this.context.state === 'running') void this.context.suspend()
  }

  close() {
    void this.context.close()
  }
}
