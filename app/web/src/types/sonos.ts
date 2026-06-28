export interface Track {
  Artist?: string;
  Title?: string;
  Album?: string;
  AlbumArtUri?: string;
  Duration?: string;
  TrackUri?: string;
}

export interface SonosState {
  uuid: string;
  model?: string;
  name: string;
  groupName?: string;
  coordinatorUuid?: string;
  volume?: Record<string, number>;
  mute?: Record<string, boolean>;
  currentTrack?: Track;
  nextTrack?: Track;
  transportState?: string;
  playmode?: string;
  bass?: number;
  treble?: number;
  ts: number;
}

export interface DeviceSummary {
  uuid: string;
  name: string;
  model: string;
  coordinator: string;
  groupName: string;
  state: SonosState | null;
}

export interface Group {
  Coordinator: string;
  Name: string;
  Members: string[];
}

export interface SSEStateEvent {
  type: 'state';
  device: string;
  state: SonosState;
}

// --- Helpers ---

export function isPlaying(state?: SonosState | null): boolean {
  return state?.transportState === 'PLAYING';
}

/** A group is "real" once it has more than one member ("Room + 2"). */
export function isGrouped(groupName?: string): boolean {
  return !!groupName && groupName.includes('+');
}

export function masterVolume(state?: SonosState | null): number {
  return state?.volume?.Master ?? 0;
}

export function isMuted(state?: SonosState | null): boolean {
  return state?.mute?.Master ?? false;
}

/** Decompose a Sonos playmode string into independent shuffle/repeat flags. */
export type RepeatMode = 'none' | 'all' | 'one';

export function shuffleFromPlaymode(mode?: string): boolean {
  return !!mode && mode.startsWith('SHUFFLE');
}

export function repeatFromPlaymode(mode?: string): RepeatMode {
  if (!mode) return 'none';
  if (mode === 'SHUFFLE') return 'all'; // SHUFFLE == shuffle + repeat all
  if (mode.includes('REPEAT_ONE')) return 'one';
  if (mode === 'REPEAT_ALL') return 'all';
  return 'none';
}

export function playmodeFor(shuffle: boolean, repeat: RepeatMode): string {
  if (shuffle) {
    if (repeat === 'one') return 'SHUFFLE_REPEAT_ONE';
    if (repeat === 'all') return 'SHUFFLE';
    return 'SHUFFLE_NOREPEAT';
  }
  if (repeat === 'one') return 'REPEAT_ONE';
  if (repeat === 'all') return 'REPEAT_ALL';
  return 'NORMAL';
}

export function nextRepeat(mode: RepeatMode): RepeatMode {
  return mode === 'none' ? 'all' : mode === 'all' ? 'one' : 'none';
}
