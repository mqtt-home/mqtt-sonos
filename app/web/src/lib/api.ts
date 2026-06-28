import type { DeviceSummary, Group, SonosState } from '@/types/sonos';

export const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

export async function fetchDevices(): Promise<DeviceSummary[]> {
  const response = await fetch(`${API_BASE}/devices`);
  if (!response.ok) throw new Error('Failed to fetch devices');
  return response.json();
}

export async function fetchGroups(): Promise<Group[]> {
  const response = await fetch(`${API_BASE}/groups`);
  if (!response.ok) throw new Error('Failed to fetch groups');
  return response.json();
}

export async function fetchState(id: string): Promise<SonosState> {
  const response = await fetch(`${API_BASE}/devices/${id}/state`);
  if (!response.ok) throw new Error('Failed to fetch state');
  return response.json();
}

async function post(id: string, action: string, body?: unknown): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${id}/${action}`, {
    method: 'POST',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) throw new Error(`Failed to ${action}`);
}

export const play = (id: string) => post(id, 'play');
export const pause = (id: string) => post(id, 'pause');
export const stop = (id: string) => post(id, 'stop');
export const next = (id: string) => post(id, 'next');
export const previous = (id: string) => post(id, 'previous');
export const leaveGroup = (id: string) => post(id, 'leave');

export const setVolume = (id: string, volume: number) => post(id, 'volume', { volume });
export const setMute = (id: string, mute: boolean) => post(id, 'mute', { mute });
export const joinGroup = (id: string, target: string) => post(id, 'join', { target });

export const sendCommand = (id: string, command: string, input?: unknown) =>
  post(id, 'command', { command, input });

export const setPlaymode = (id: string, mode: string) => sendCommand(id, 'playmode', mode);
