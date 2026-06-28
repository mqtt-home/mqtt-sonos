import { useEffect, useRef, useState } from 'react';
import {
  Play, Pause, SkipBack, SkipForward, Volume2, VolumeX, Volume1,
  Music, Users, LogOut, Shuffle, Repeat, Repeat1, Plus, Check,
} from 'lucide-react';
import {
  play, pause, next, previous, setVolume, setMute, leaveGroup, joinGroup, setPlaymode,
} from '@/lib/api';
import {
  isPlaying, isGrouped, masterVolume, isMuted,
  shuffleFromPlaymode, repeatFromPlaymode, playmodeFor, nextRepeat,
} from '@/types/sonos';
import type { DeviceSummary, SonosState } from '@/types/sonos';
import { cn } from '@/lib/utils';

interface Props {
  device: DeviceSummary;
  state?: SonosState | null;
  // Coordinator state, used to render now-playing for non-coordinator members.
  coordinatorState?: SonosState | null;
  // Other rooms this device can join (their uuid + name).
  joinTargets: { uuid: string; name: string }[];
}

function transportLabel(s?: string): { text: string; cls: string } {
  switch (s) {
    case 'PLAYING': return { text: 'Playing', cls: 'bg-green-500/15 text-green-500' };
    case 'PAUSED_PLAYBACK': return { text: 'Paused', cls: 'bg-amber-500/15 text-amber-500' };
    case 'TRANSITIONING': return { text: 'Loading', cls: 'bg-blue-500/15 text-blue-500' };
    case 'STOPPED': return { text: 'Stopped', cls: 'bg-muted text-muted-foreground' };
    default: return { text: 'Idle', cls: 'bg-muted text-muted-foreground' };
  }
}

export function DeviceCard({ device, state, coordinatorState, joinTargets }: Props) {
  const uuid = device.uuid;
  // Transport / playmode act on the coordinator of the device's group.
  const coordId = device.coordinator || uuid;
  const isCoordinator = !device.coordinator || device.coordinator === uuid;
  const grouped = isGrouped(device.groupName);

  // Now-playing comes from the coordinator (group leader holds the track info).
  const trackSource = isCoordinator ? state : (coordinatorState ?? state);
  const track = trackSource?.currentTrack;
  const playing = isPlaying(trackSource);
  const transport = transportLabel(trackSource?.transportState);

  const playmode = trackSource?.playmode;
  const shuffle = shuffleFromPlaymode(playmode);
  const repeat = repeatFromPlaymode(playmode);

  const muted = isMuted(state);

  // --- Volume (local + debounced POST) ---
  const [vol, setVol] = useState(masterVolume(state));
  const draggingRef = useRef(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!draggingRef.current) setVol(masterVolume(state));
  }, [state]);

  const onVolChange = (v: number) => {
    draggingRef.current = true;
    setVol(v);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setVolume(uuid, v).catch(console.error);
      draggingRef.current = false;
    }, 250);
  };

  const [busy, setBusy] = useState(false);
  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    try { await fn(); } catch (e) { console.error(e); } finally { setBusy(false); }
  };

  const [albumArtError, setAlbumArtError] = useState(false);
  useEffect(() => { setAlbumArtError(false); }, [track?.AlbumArtUri]);

  const [showJoin, setShowJoin] = useState(false);

  const VolIcon = muted ? VolumeX : vol < 40 ? Volume1 : Volume2;
  const hasTrack = !!(track && (track.Title || track.Artist));

  return (
    <div className="bg-card rounded-xl border border-border p-4 flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="text-lg font-semibold text-foreground truncate">{device.name}</h2>
          <p className="text-xs text-muted-foreground truncate">{device.model}</p>
        </div>
        <span className={cn('shrink-0 text-xs font-medium px-2 py-1 rounded-full', transport.cls)}>
          {transport.text}
        </span>
      </div>

      {/* Now playing */}
      <div className="flex items-center gap-3">
        <div className="h-16 w-16 shrink-0 rounded-lg bg-muted overflow-hidden flex items-center justify-center">
          {track?.AlbumArtUri && !albumArtError ? (
            <img
              src={track.AlbumArtUri}
              alt=""
              className="h-full w-full object-cover"
              onError={() => setAlbumArtError(true)}
            />
          ) : (
            <Music className="h-6 w-6 text-muted-foreground" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          {hasTrack ? (
            <>
              <p className="text-sm font-medium text-foreground truncate">{track?.Title || 'Unknown title'}</p>
              <p className="text-xs text-muted-foreground truncate">{track?.Artist || 'Unknown artist'}</p>
              {track?.Album && <p className="text-xs text-muted-foreground truncate">{track.Album}</p>}
            </>
          ) : (
            <p className="text-sm text-muted-foreground">Nothing playing</p>
          )}
        </div>
      </div>

      {/* Transport controls */}
      <div className="flex items-center justify-center gap-2">
        <button
          onClick={() => run(() => previous(coordId))}
          disabled={busy}
          className="p-2 rounded-lg hover:bg-accent transition-colors touch-target flex items-center justify-center disabled:opacity-50"
          aria-label="Previous"
        >
          <SkipBack className="h-5 w-5 text-foreground" />
        </button>
        <button
          onClick={() => run(() => (playing ? pause(coordId) : play(coordId)))}
          disabled={busy}
          className="p-3 rounded-full bg-primary hover:opacity-90 transition-opacity touch-target flex items-center justify-center disabled:opacity-50"
          aria-label={playing ? 'Pause' : 'Play'}
        >
          {playing
            ? <Pause className="h-6 w-6 text-primary-foreground" />
            : <Play className="h-6 w-6 text-primary-foreground" />}
        </button>
        <button
          onClick={() => run(() => next(coordId))}
          disabled={busy}
          className="p-2 rounded-lg hover:bg-accent transition-colors touch-target flex items-center justify-center disabled:opacity-50"
          aria-label="Next"
        >
          <SkipForward className="h-5 w-5 text-foreground" />
        </button>

        <div className="w-px h-6 bg-border mx-1" />

        <button
          onClick={() => run(() => setPlaymode(coordId, playmodeFor(!shuffle, repeat)))}
          disabled={busy}
          className={cn('p-2 rounded-lg hover:bg-accent transition-colors flex items-center justify-center',
            shuffle ? 'text-primary' : 'text-muted-foreground')}
          aria-label="Toggle shuffle"
          title="Shuffle"
        >
          <Shuffle className="h-4 w-4" />
        </button>
        <button
          onClick={() => run(() => setPlaymode(coordId, playmodeFor(shuffle, nextRepeat(repeat))))}
          disabled={busy}
          className={cn('p-2 rounded-lg hover:bg-accent transition-colors flex items-center justify-center',
            repeat !== 'none' ? 'text-primary' : 'text-muted-foreground')}
          aria-label="Cycle repeat"
          title={`Repeat: ${repeat}`}
        >
          {repeat === 'one' ? <Repeat1 className="h-4 w-4" /> : <Repeat className="h-4 w-4" />}
        </button>
      </div>

      {/* Volume */}
      <div className="flex items-center gap-3">
        <button
          onClick={() => run(() => setMute(uuid, !muted))}
          disabled={busy}
          className="p-1.5 rounded-lg hover:bg-accent transition-colors shrink-0"
          aria-label={muted ? 'Unmute' : 'Mute'}
        >
          <VolIcon className={cn('h-5 w-5', muted ? 'text-red-500' : 'text-foreground')} />
        </button>
        <input
          type="range"
          min={0}
          max={100}
          value={vol}
          onChange={(e) => onVolChange(Number(e.target.value))}
          className="volume flex-1"
          aria-label="Volume"
        />
        <span className="text-xs text-muted-foreground tabular-nums w-8 text-right">{vol}</span>
      </div>

      {/* Group membership */}
      <div className="border-t border-border pt-3">
        {grouped ? (
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-1.5 min-w-0">
              <Users className="h-4 w-4 text-primary shrink-0" />
              <span className="text-xs text-muted-foreground truncate">
                {device.groupName}
                {isCoordinator
                  ? <span className="ml-1 text-primary font-medium">(leader)</span>
                  : null}
              </span>
            </div>
            <button
              onClick={() => run(() => leaveGroup(uuid))}
              disabled={busy}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded-lg hover:bg-accent transition-colors text-muted-foreground shrink-0"
            >
              <LogOut className="h-3.5 w-3.5" /> Leave
            </button>
          </div>
        ) : (
          <div className="relative">
            <button
              onClick={() => setShowJoin(v => !v)}
              disabled={busy || joinTargets.length === 0}
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded-lg hover:bg-accent transition-colors text-muted-foreground disabled:opacity-50"
            >
              <Plus className="h-3.5 w-3.5" /> Join a group
            </button>
            {showJoin && joinTargets.length > 0 && (
              <div className="absolute z-10 mt-1 w-48 bg-card border border-border rounded-lg shadow-lg py-1 max-h-56 overflow-auto">
                {joinTargets.map(t => (
                  <button
                    key={t.uuid}
                    onClick={() => { setShowJoin(false); run(() => joinGroup(uuid, t.uuid)); }}
                    className="w-full text-left px-3 py-2 text-sm text-foreground hover:bg-accent transition-colors flex items-center justify-between gap-2"
                  >
                    <span className="truncate">{t.name}</span>
                    <Check className="h-3.5 w-3.5 text-transparent" />
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
