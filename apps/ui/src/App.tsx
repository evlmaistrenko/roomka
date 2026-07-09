import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import {
  Maximize,
  Minimize,
  MonitorUp,
  MonitorX,
  Settings,
  Volume1,
  Volume2,
  VolumeX,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { SettingsDialog } from '@/components/settings-dialog'
import { useBroadcast } from '@/hooks/use-broadcast'
import {
  loadBroadcastConfig,
  saveBroadcastConfig,
  type BroadcastConfig,
} from '@/lib/broadcast-config'
import { cn } from '@/lib/utils'

type TileId = number | 'local'

function App() {
  const {
    senders,
    isSharing,
    localStream,
    error,
    startSharing,
    stopSharing,
    setPeerPlaying,
    setVolume: setBroadcastVolume,
    getStream,
  } = useBroadcast()

  const [active, setActive] = useState<TileId | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [volume, setVolume] = useState(1)

  const changeVolume = (value: number) => {
    setVolume(value)
    setBroadcastVolume(value)
  }
  const [broadcastConfig, setBroadcastConfig] =
    useState<BroadcastConfig>(loadBroadcastConfig)

  const updateConfig = useCallback((config: BroadcastConfig) => {
    setBroadcastConfig(config)
    saveBroadcastConfig(config)
  }, [])

  // Stable identity so SettingsDialog's Escape/focus-trap effects don't tear
  // down and re-subscribe on every App re-render while the dialog is open.
  const closeSettings = useCallback(() => setSettingsOpen(false), [])

  const streamFor = (id: TileId): MediaStream | null =>
    id === 'local' ? localStream : getStream(id)

  const tiles: { id: TileId; label: string }[] = [
    ...(localStream ? [{ id: 'local' as TileId, label: 'You' }] : []),
    ...senders.map((id) => ({ id: id as TileId, label: `Peer ${id.toString(16)}` })),
  ]

  // Resolve the selection against what's currently live — if the active
  // broadcast went away (sender pruned or local sharing stopped) the main stage
  // falls back to its empty state without having to reset the stored id.
  const activeStream = active === null ? null : streamFor(active)
  const resolvedActive = activeStream === null ? null : active

  // Selecting a broadcast promotes it to the main stage. The click is also the
  // user gesture that lets the selected peer's audio resume (autoplay policy):
  // start the newly selected peer's audio and stop the previously selected one.
  const select = (id: TileId) => {
    if (typeof active === 'number' && active !== id) setPeerPlaying(active, false)
    if (typeof id === 'number') setPeerPlaying(id, true)
    setActive(id)
  }

  return (
    <div className="flex h-dvh flex-col">
      {error && (
        <p className="border-b bg-destructive/10 px-4 py-2 text-sm text-destructive">
          {error}
        </p>
      )}

      <div className="flex min-h-0 flex-1">
        <MainStage
          stream={activeStream}
          hasAudio={typeof resolvedActive === 'number'}
          volume={volume}
          onVolumeChange={changeVolume}
          empty={
            tiles.length > 0
              ? 'Select a stream on the right to watch it here.'
              : 'No active streams. Click “Share screen”, or open this page in another tab and share there.'
          }
        />

        {tiles.length > 0 && (
          <aside className="flex w-64 shrink-0 flex-col gap-2 overflow-y-auto border-l p-2">
            {tiles.map((tile) => (
              <Thumbnail
                key={String(tile.id)}
                stream={streamFor(tile.id)}
                label={tile.label}
                active={resolvedActive === tile.id}
                onSelect={() => select(tile.id)}
              />
            ))}
          </aside>
        )}
      </div>

      <footer className="flex items-center gap-2 border-t px-4 py-3">
        <div className="flex-1" />
        {isSharing ? (
          <Button variant="destructive" onClick={stopSharing}>
            <MonitorX /> Stop sharing
          </Button>
        ) : (
          <Button onClick={() => void startSharing(broadcastConfig)}>
            <MonitorUp /> Share screen
          </Button>
        )}
        <div className="flex flex-1 items-center justify-end gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setSettingsOpen(true)}
            aria-label="Settings"
            title="Settings"
          >
            <Settings />
          </Button>
        </div>
      </footer>

      <SettingsDialog
        open={settingsOpen}
        config={broadcastConfig}
        onChange={updateConfig}
        onClose={closeSettings}
      />
    </div>
  )
}

// The main stage renders the selected broadcast fitted (object-contain) inside a
// fixed, non-scrolling area, with a Fullscreen API toggle overlaid on hover.
function MainStage({
  stream,
  hasAudio,
  volume,
  onVolumeChange,
  empty,
}: {
  stream: MediaStream | null
  hasAudio: boolean
  volume: number
  onVolumeChange: (value: number) => void
  empty: ReactNode
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const videoRef = useRef<HTMLVideoElement>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)

  useEffect(() => {
    const video = videoRef.current
    if (video) video.srcObject = stream
  }, [stream])

  useEffect(() => {
    const onChange = () =>
      setIsFullscreen(document.fullscreenElement === containerRef.current)
    document.addEventListener('fullscreenchange', onChange)
    return () => document.removeEventListener('fullscreenchange', onChange)
  }, [])

  const toggleFullscreen = () => {
    if (document.fullscreenElement) {
      void document.exitFullscreen()
    } else {
      void containerRef.current?.requestFullscreen().catch(() => undefined)
    }
  }

  return (
    <div ref={containerRef} className="group relative min-h-0 flex-1 bg-muted">
      {stream ? (
        <>
          {/* Video-only stream (audio is played via Web Audio), so muted just
              guarantees autoplay. object-contain fits it without cropping. */}
          <video
            ref={videoRef}
            autoPlay
            muted
            playsInline
            className="h-full w-full object-contain"
          />
          <div className="absolute right-3 top-3 flex items-center gap-2 opacity-0 transition group-hover:opacity-100">
            {hasAudio && (
              <VolumeControl volume={volume} onChange={onVolumeChange} />
            )}
            <Button
              variant="secondary"
              size="icon"
              onClick={toggleFullscreen}
              aria-label={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
              title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
            >
              {isFullscreen ? <Minimize /> : <Maximize />}
            </Button>
          </div>
        </>
      ) : (
        <div className="flex h-full w-full items-center justify-center p-6 text-center text-sm text-muted-foreground">
          {empty}
        </div>
      )}
    </div>
  )
}

// Mute button + slider governing the master playback volume of the broadcast
// on the main stage. Audio plays through Web Audio, so this drives a gain node
// (see useBroadcast.setVolume), not the <video> element's volume.
function VolumeControl({
  volume,
  onChange,
}: {
  volume: number
  onChange: (value: number) => void
}) {
  const beforeMute = useRef(1)
  const muted = volume === 0
  const Icon = muted ? VolumeX : volume < 0.5 ? Volume1 : Volume2

  const toggleMute = () => {
    if (muted) {
      onChange(beforeMute.current || 1)
    } else {
      beforeMute.current = volume
      onChange(0)
    }
  }

  return (
    <div className="flex h-9 items-center gap-1 rounded-md bg-secondary px-1 text-secondary-foreground shadow-xs">
      <Button
        variant="ghost"
        size="icon"
        className="size-7"
        onClick={toggleMute}
        aria-label={muted ? 'Unmute' : 'Mute'}
        title={muted ? 'Unmute' : 'Mute'}
      >
        <Icon />
      </Button>
      <input
        type="range"
        min={0}
        max={1}
        step={0.01}
        value={volume}
        onChange={(event) => onChange(Number(event.target.value))}
        aria-label="Volume"
        className="mr-1 h-1 w-24 cursor-pointer accent-primary"
      />
    </div>
  )
}

// A live preview of a broadcast in the right-hand column. Autoplays muted (the
// stream has no audio track) so it needs no user gesture; clicking it selects
// the broadcast for the main stage.
function Thumbnail({
  stream,
  label,
  active,
  onSelect,
}: {
  stream: MediaStream | null
  label: string
  active: boolean
  onSelect: () => void
}) {
  const ref = useRef<HTMLVideoElement>(null)
  useEffect(() => {
    const video = ref.current
    if (!video) return
    video.muted = true
    video.srcObject = stream
  }, [stream])

  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        'relative block w-full cursor-pointer overflow-hidden rounded-lg border bg-muted text-left transition',
        active ? 'ring-2 ring-primary' : 'hover:border-primary/50',
      )}
      aria-pressed={active}
    >
      <video
        ref={ref}
        autoPlay
        muted
        playsInline
        className="block aspect-video w-full object-cover"
      />
      <span className="absolute bottom-1 left-1 rounded bg-black/60 px-1.5 py-0.5 text-xs text-white">
        {label}
      </span>
    </button>
  )
}

export default App
