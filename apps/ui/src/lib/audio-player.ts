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

    const now = this.context.currentTime
    if (this.nextStartTime < now) {
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
