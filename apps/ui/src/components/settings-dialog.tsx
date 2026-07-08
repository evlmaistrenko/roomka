import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { KeyRound, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { ThemeToggle } from '@/components/theme-toggle'
import { cn } from '@/lib/utils'
import { isPresetSupported, VIDEO_PRESETS } from '@/lib/video-presets'

// A small self-contained modal (no extra dependency) for broadcast settings.
// For now it only picks the video codec/quality preset; more settings can slot
// into the same body later.
export function SettingsDialog({
  open,
  presetId,
  onSelect,
  onClose,
}: {
  open: boolean
  presetId: string
  onSelect: (id: string) => void
  onClose: () => void
}) {
  const navigate = useNavigate()
  const dialogRef = useRef<HTMLDivElement>(null)
  // Preset ids the device can actually encode. Unknown (before the probe
  // resolves) is treated as supported so nothing flickers disabled.
  const [supported, setSupported] = useState<Record<string, boolean>>({})

  useEffect(() => {
    if (!open) return
    let cancelled = false
    void Promise.all(
      VIDEO_PRESETS.map(
        async (preset) => [preset.id, await isPresetSupported(preset)] as const,
      ),
    ).then((entries) => {
      if (!cancelled) setSupported(Object.fromEntries(entries))
    })
    return () => {
      cancelled = true
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  // Focus management: move focus into the dialog on open, keep Tab within it,
  // and restore focus to whatever was focused (the trigger) on close.
  useEffect(() => {
    if (!open) return
    const dialog = dialogRef.current
    if (!dialog) return
    const previouslyFocused = document.activeElement as HTMLElement | null

    const focusable = () =>
      Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      )

    dialog.focus()

    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Tab') return
      const items = focusable()
      if (items.length === 0) {
        event.preventDefault()
        return
      }
      const first = items[0]
      const last = items[items.length - 1]
      const active = document.activeElement
      const inside = active instanceof HTMLElement && items.includes(active)
      if (event.shiftKey && (!inside || active === first)) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && (!inside || active === last)) {
        event.preventDefault()
        first.focus()
      }
    }

    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      previouslyFocused?.focus?.()
    }
  }, [open])

  if (!open) return null

  const families = [...new Set(VIDEO_PRESETS.map((preset) => preset.family))]

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="Settings"
        tabIndex={-1}
        className="w-full max-w-md rounded-lg border bg-background p-4 shadow-lg outline-none"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">Settings</h2>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={onClose}
            aria-label="Close"
          >
            <X />
          </Button>
        </div>

        <p className="mb-2 text-sm font-medium">Broadcast codec</p>
        <div className="space-y-3">
          {families.map((family) => (
            <div key={family}>
              <p className="mb-1 text-xs text-muted-foreground">{family}</p>
              <div className="grid grid-cols-3 gap-2">
                {VIDEO_PRESETS.filter((preset) => preset.family === family).map(
                  (preset) => {
                    const available = supported[preset.id] !== false
                    const selected = preset.id === presetId
                    return (
                      <button
                        key={preset.id}
                        type="button"
                        disabled={!available}
                        onClick={() => onSelect(preset.id)}
                        title={
                          available ? undefined : 'Not supported on this device'
                        }
                        className={cn(
                          'rounded-md border px-2 py-1.5 text-sm transition',
                          available
                            ? 'cursor-pointer'
                            : 'cursor-not-allowed opacity-40',
                          selected
                            ? 'border-primary bg-primary/10 font-medium'
                            : 'hover:border-primary/50',
                        )}
                      >
                        {preset.label}
                      </button>
                    )
                  },
                )}
              </div>
            </div>
          ))}
        </div>

        <p className="mt-4 text-xs text-muted-foreground">
          Applies to your next screen share. Receivers detect the codec
          automatically.
        </p>

        <div className="mt-4 flex items-center gap-2 border-t pt-4">
          <p className="text-sm font-medium">Theme</p>
          <ThemeToggle />
        </div>

        <div className="mt-4 border-t pt-4">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              onClose()
              navigate('/import')
            }}
          >
            <KeyRound /> Replace settings file
          </Button>
        </div>
      </div>
    </div>
  )
}
