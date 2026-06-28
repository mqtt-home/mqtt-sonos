import { useEffect, useMemo, useState } from 'react';
import { Sun, Moon, Wifi, WifiOff, Speaker, RefreshCw } from 'lucide-react';
import { useSSE } from '@/hooks/useSSE';
import { fetchDevices } from '@/lib/api';
import { useTheme } from '@/contexts/ThemeContext';
import { DeviceCard } from '@/components/DeviceCard';
import type { DeviceSummary, SonosState } from '@/types/sonos';

export function App() {
  const [devices, setDevices] = useState<DeviceSummary[]>([]);
  const [loaded, setLoaded] = useState(false);
  const { states, isConnected, error, reconnect } = useSSE();
  const { theme, toggleTheme } = useTheme();

  const load = () => {
    fetchDevices()
      .then(setDevices)
      .catch(console.error)
      .finally(() => setLoaded(true));
  };

  useEffect(() => { load(); }, []);

  // Merge live SSE state onto each device. The SSE state also carries the
  // current coordinator/group, so prefer it for grouping when available.
  const liveDevices = useMemo(() => {
    return devices.map(d => {
      const live = states[d.uuid] ?? d.state ?? undefined;
      return {
        ...d,
        coordinator: live?.coordinatorUuid ?? d.coordinator,
        groupName: live?.groupName ?? d.groupName,
        state: live ?? null,
      } as DeviceSummary;
    });
  }, [devices, states]);

  const stateFor = (uuid: string): SonosState | null =>
    states[uuid] ?? devices.find(d => d.uuid === uuid)?.state ?? null;

  return (
    <div className="min-h-screen bg-background p-4 md:p-8">
      <div className="max-w-6xl mx-auto">
        {/* Header */}
        <header className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-2">
            <Speaker className="h-6 w-6 text-primary" />
            <h1 className="text-2xl font-bold text-foreground">Sonos Control</h1>
          </div>
          <div className="flex items-center gap-1">
            <button onClick={load} className="p-2 rounded-lg hover:bg-accent transition-colors" aria-label="Refresh" title="Refresh devices">
              <RefreshCw className="h-5 w-5 text-foreground" />
            </button>
            <div className="p-2" title={isConnected ? 'Connected' : 'Disconnected'}>
              {isConnected
                ? <Wifi className="h-5 w-5 text-green-500" />
                : <WifiOff className="h-5 w-5 text-red-500 cursor-pointer" onClick={reconnect} />}
            </div>
            <button onClick={toggleTheme} className="p-2 rounded-lg hover:bg-accent transition-colors" aria-label="Toggle theme">
              {theme === 'dark' ? <Sun className="h-5 w-5 text-foreground" /> : <Moon className="h-5 w-5 text-foreground" />}
            </button>
          </div>
        </header>

        {error && (
          <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-500 text-sm">
            {error}
            <button onClick={reconnect} className="ml-2 underline hover:no-underline">Retry</button>
          </div>
        )}

        {!loaded ? (
          <div className="text-muted-foreground text-center py-16">Loading speakers...</div>
        ) : liveDevices.length === 0 ? (
          <div className="text-muted-foreground text-center py-16">No Sonos speakers found.</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {liveDevices.map(device => {
              const coordId = device.coordinator || device.uuid;
              const joinTargets = liveDevices
                .filter(d => (d.coordinator || d.uuid) !== coordId)
                .map(d => ({ uuid: d.uuid, name: d.name }));
              return (
                <DeviceCard
                  key={device.uuid}
                  device={device}
                  state={device.state}
                  coordinatorState={stateFor(coordId)}
                  joinTargets={joinTargets}
                />
              );
            })}
          </div>
        )}

        <div className="mt-8 text-center text-xs text-muted-foreground">mqtt-sonos</div>
      </div>
    </div>
  );
}
