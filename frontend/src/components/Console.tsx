import React, { useEffect, useRef, useState } from 'react';
import { Virtuoso } from 'react-virtuoso';
import type { VirtuosoHandle } from 'react-virtuoso';

interface ConsoleProps {
  serverId: string;
  onClose?: () => void;
}

const Console: React.FC<ConsoleProps> = ({ serverId, onClose }) => {
  const [logs, setLogs] = useState<string[]>([]);
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const [atBottom, setAtBottom] = useState(true);
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = import.meta.env.VITE_API_URL 
      ? import.meta.env.VITE_API_URL.replace(/^https?:\/\//, '') 
      : window.location.host;
    const token = localStorage.getItem('token');
    
    const wsUrl = `${protocol}//${host}/api/logs/${serverId}?token=${token}`;
    const socket = new WebSocket(wsUrl);
    socketRef.current = socket;

    socket.onmessage = (event) => {
      const newLog = event.data;
      setLogs((prev) => [...prev, newLog]);
    };

    socket.onclose = () => {
      setLogs((prev) => [...prev, '--- Connection closed ---']);
    };

    socket.onerror = (error) => {
      console.error('WebSocket error:', error);
      setLogs((prev) => [...prev, '--- WebSocket error ---']);
    };

    return () => {
      socket.close();
    };
  }, [serverId]);

  useEffect(() => {
    if (atBottom && virtuosoRef.current) {
      virtuosoRef.current.scrollToIndex({
        index: logs.length - 1,
        behavior: 'auto',
      });
    }
  }, [logs, atBottom]);

  return (
    <div className="flex flex-col h-[500px] w-full glass rounded-xl overflow-hidden border border-glass-border">
      <div className="flex items-center justify-between px-4 py-2 bg-black/20 border-b border-glass-border">
        <h3 className="text-sm font-semibold flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse"></span>
          Live Console - {serverId.substring(0, 12)}
        </h3>
        {onClose && (
          <button 
            onClick={onClose}
            className="text-gray-400 hover:text-white transition-colors"
          >
            <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        )}
      </div>
      <div className="flex-1 bg-black/40 font-mono text-xs p-2 overflow-hidden">
        <Virtuoso
          ref={virtuosoRef}
          data={logs}
          atBottomStateChange={setAtBottom}
          initialTopMostItemIndex={logs.length - 1}
          itemContent={(index, log) => (
            <div className="py-0.5 whitespace-pre-wrap break-all border-l-2 border-transparent hover:border-accent/30 hover:bg-white/5 px-2">
              <span className="text-gray-500 mr-2 select-none">[{index + 1}]</span>
              <span className="text-gray-300">{log}</span>
            </div>
          )}
          followOutput="auto"
          className="custom-scrollbar"
        />
      </div>
      <div className="px-4 py-1 text-[10px] text-gray-500 bg-black/20 border-top border-glass-border flex justify-between">
        <span>{logs.length} lines rendered</span>
        <span>{atBottom ? 'Auto-scrolling enabled' : 'Auto-scrolling paused'}</span>
      </div>
    </div>
  );
};

export default Console;
