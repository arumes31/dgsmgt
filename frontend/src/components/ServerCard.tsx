import React, { useState, useEffect, useRef } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Play, Square, RefreshCw, Terminal, Activity, Loader2, Copy, Check, Settings } from 'lucide-react';
import api from '../api';
import type { ServerWithPerms } from '../api';
import Console from './Console';
import MetricsChart from './MetricsChart';
import Modal from './Modal';

interface ServerCardProps {
  server: ServerWithPerms;
  onAction?: () => void;
}

interface ContextMenuState {
  x: number;
  y: number;
  visible: boolean;
}

const ServerCard: React.FC<ServerCardProps> = ({ server, onAction }) => {
  const [status, setStatus] = useState<any>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [isConsoleOpen, setIsConsoleOpen] = useState(false);
  const [isMetricsOpen, setIsMetricsOpen] = useState(false);
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({ x: 0, y: 0, visible: false });
  const cardRef = useRef<HTMLDivElement>(null);

  // Mock IP and Port
  const ip = '127.0.0.1';
  let port = '25565';
  try {
    const config = JSON.parse(server.ConfigJSON || '{}');
    if (config.ports && config.ports[0]) {
      port = config.ports[0].split(':')[0];
    }
  } catch (e) {}

  const fullAddress = `${ip}:${port}`;

  const fetchStatus = async () => {
    try {
      const response = await api.get(`/api/status/${server.ContainerID}`);
      setStatus(response.data);
    } catch (error) {
      console.error('Failed to fetch status:', error);
    }
  };

  useEffect(() => {
    fetchStatus();
    const interval = setInterval(fetchStatus, 10000);
    return () => clearInterval(interval);
  }, [server.ContainerID]);

  const handleAction = async (action: string) => {
    setActionLoading(action);
    setContextMenu({ ...contextMenu, visible: false });
    try {
      await api.post(`/api/action/${server.ContainerID}/${action}`);
      setTimeout(() => {
        fetchStatus();
        if (onAction) onAction();
      }, 2000);
    } catch (error) {
      console.error(`Failed to ${action} server:`, error);
    } finally {
      setActionLoading(null);
    }
  };

  const copyToClipboard = () => {
    navigator.clipboard.writeText(fullAddress);
    setCopied(true);
    setContextMenu({ ...contextMenu, visible: false });
    setTimeout(() => setCopied(false), 2000);
  };

  const isRunning = status?.status === 'running';

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      visible: true,
    });
  };

  useEffect(() => {
    const handleClick = () => setContextMenu({ ...contextMenu, visible: false });
    window.addEventListener('click', handleClick);
    return () => window.removeEventListener('click', handleClick);
  }, [contextMenu]);

  return (
    <>
      <motion.div
        ref={cardRef}
        initial={{ opacity: 0, scale: 0.95 }}
        animate={{ opacity: 1, scale: 1 }}
        onContextMenu={handleContextMenu}
        className="glass p-6 rounded-3xl border border-white/10 space-y-4 hover:shadow-2xl transition-all relative group"
      >
        <div className="flex justify-between items-start">
          <div className="flex items-center space-x-3">
            <div className={`w-3 h-3 rounded-full ${isRunning ? 'bg-green-500 animate-pulse' : 'bg-red-500 shadow-[0_0_10px_rgba(239,68,68,0.5)]'}`} />
            <h3 className="text-xl font-bold">{server.Name}</h3>
          </div>
          <div className="flex items-center space-x-2">
            <div className="text-xs font-mono opacity-50 bg-foreground/5 px-2 py-1 rounded-md">
              {server.ContainerID.substring(0, 12)}
            </div>
            <button className="opacity-0 group-hover:opacity-100 transition-opacity p-1 hover:bg-white/10 rounded-lg">
               <Settings size={14} className="text-gray-400" />
            </button>
          </div>
        </div>

        <div className="flex items-center justify-between p-3 rounded-2xl bg-foreground/5 border border-white/5">
          <div className="flex flex-col">
            <p className="text-[10px] uppercase tracking-wider opacity-60 font-bold mb-1">Server Address</p>
            <p className="font-mono text-sm">{fullAddress}</p>
          </div>
          <button
            onClick={copyToClipboard}
            className="p-2 rounded-lg hover:bg-foreground/10 transition-colors cursor-pointer"
            title="Copy IP:Port"
          >
            {copied ? <Check size={16} className="text-green-500" /> : <Copy size={16} className="opacity-60" />}
          </button>
        </div>

        <div className="grid grid-cols-2 gap-4 py-2 border-t border-white/10">
          <div className="space-y-1 text-left">
            <p className="text-[10px] uppercase tracking-wider opacity-60 font-bold">Status</p>
            <p className={`text-sm font-medium ${isRunning ? 'text-green-500' : 'opacity-40'}`}>
              {status?.status || 'Offline'}
            </p>
          </div>
          <div className="space-y-1 text-right">
            <p className="text-[10px] uppercase tracking-wider opacity-60 font-bold">Image</p>
            <p className="text-sm opacity-80 truncate" title={server.Image}>{server.Image}</p>
          </div>
        </div>

        <div className="flex flex-wrap gap-2 pt-2">
          {server.can_start && (
            <button
              onClick={() => handleAction('start')}
              disabled={isRunning || actionLoading !== null}
              className="flex-1 flex items-center justify-center space-x-2 py-2 px-3 bg-green-500/10 hover:bg-green-500/20 text-green-500 rounded-xl transition-all disabled:opacity-30 disabled:cursor-not-allowed"
            >
              {actionLoading === 'start' ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
              <span className="text-sm font-bold">Start</span>
            </button>
          )}
          {server.can_stop && (
            <button
              onClick={() => handleAction('stop')}
              disabled={!isRunning || actionLoading !== null}
              className="flex-1 flex items-center justify-center space-x-2 py-2 px-3 bg-red-500/10 hover:bg-red-500/20 text-red-500 rounded-xl transition-all disabled:opacity-30 disabled:cursor-not-allowed"
            >
              {actionLoading === 'stop' ? <Loader2 size={16} className="animate-spin" /> : <Square size={16} />}
              <span className="text-sm font-bold">Stop</span>
            </button>
          )}
          <div className="flex gap-2">
            {server.can_restart && (
              <button
                onClick={() => handleAction('restart')}
                disabled={actionLoading !== null}
                title="Restart"
                className="flex items-center justify-center p-2 bg-yellow-500/10 hover:bg-yellow-500/20 text-yellow-500 rounded-xl transition-all disabled:opacity-30"
              >
                {actionLoading === 'restart' ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
              </button>
            )}
            {server.can_view_logs && (
              <button 
                onClick={() => setIsConsoleOpen(true)}
                title="View Logs"
                className="flex items-center justify-center p-2 bg-blue-500/10 hover:bg-blue-500/20 text-blue-500 rounded-xl transition-all"
              >
                <Terminal size={16} />
              </button>
            )}
            <button 
              onClick={() => setIsMetricsOpen(true)}
              title="View Metrics"
              className="flex items-center justify-center p-2 bg-purple-500/10 hover:bg-purple-500/20 text-purple-500 rounded-xl transition-all"
            >
              <Activity size={16} />
            </button>
          </div>
        </div>

        {isRunning && status?.stats && (
          <div className="pt-4 mt-4 border-t border-white/10 grid grid-cols-2 gap-4">
            <div className="flex items-center space-x-2 text-[11px] opacity-60">
              <Activity size={12} className="text-blue-500" />
              <span>CPU: {status.stats.cpu_usage.toFixed(1)}%</span>
            </div>
            <div className="flex items-center space-x-2 text-[11px] opacity-60 justify-end">
              <Activity size={12} className="text-purple-500" />
              <span>RAM: {(status.stats.memory_usage / 1024 / 1024).toFixed(0)}MB</span>
            </div>
          </div>
        )}
      </motion.div>

      {/* Context Menu */}
      <AnimatePresence>
        {contextMenu.visible && (
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }}
            style={{ top: contextMenu.y, left: contextMenu.x }}
            className="fixed z-[100] w-48 glass rounded-2xl border border-white/10 shadow-2xl py-2 overflow-hidden"
          >
            <div className="px-3 py-1.5 text-[10px] uppercase tracking-widest text-gray-400 font-bold border-b border-white/5 mb-1">
              Actions
            </div>
            {server.can_start && !isRunning && (
              <button 
                onClick={() => handleAction('start')}
                className="w-full flex items-center space-x-3 px-4 py-2 hover:bg-green-500/10 text-green-500 transition-colors text-sm font-medium"
              >
                <Play size={14} />
                <span>Start Server</span>
              </button>
            )}
            {server.can_stop && isRunning && (
              <button 
                onClick={() => handleAction('stop')}
                className="w-full flex items-center space-x-3 px-4 py-2 hover:bg-red-500/10 text-red-500 transition-colors text-sm font-medium"
              >
                <Square size={14} />
                <span>Stop Server</span>
              </button>
            )}
            {server.can_restart && (
              <button 
                onClick={() => handleAction('restart')}
                className="w-full flex items-center space-x-3 px-4 py-2 hover:bg-yellow-500/10 text-yellow-500 transition-colors text-sm font-medium"
              >
                <RefreshCw size={14} />
                <span>Restart</span>
              </button>
            )}
            <div className="h-px bg-white/5 my-1" />
            <button 
              onClick={copyToClipboard}
              className="w-full flex items-center space-x-3 px-4 py-2 hover:bg-white/10 text-white transition-colors text-sm"
            >
              <Copy size={14} className="opacity-60" />
              <span>Copy IP Address</span>
            </button>
            <button 
              onClick={() => setIsMetricsOpen(true)}
              className="w-full flex items-center space-x-3 px-4 py-2 hover:bg-white/10 text-white transition-colors text-sm"
            >
              <Activity size={14} className="opacity-60" />
              <span>Metrics</span>
            </button>
            <button 
              onClick={() => setIsConsoleOpen(true)}
              className="w-full flex items-center space-x-3 px-4 py-2 hover:bg-white/10 text-white transition-colors text-sm"
            >
              <Terminal size={14} className="opacity-60" />
              <span>Web Console</span>
            </button>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Modals */}
      <Modal 
        isOpen={isConsoleOpen} 
        onClose={() => setIsConsoleOpen(false)} 
        title={`Console - ${server.Name}`}
        maxWidth="max-w-5xl"
      >
        <Console serverId={server.ContainerID} />
      </Modal>

      <Modal 
        isOpen={isMetricsOpen} 
        onClose={() => setIsMetricsOpen(false)} 
        title={`Live Metrics - ${server.Name}`}
      >
        <div className="space-y-6">
          <MetricsChart serverId={server.ContainerID} />
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
             <div className="bg-white/5 p-4 rounded-2xl border border-white/5">
                <p className="text-[10px] text-gray-400 font-bold uppercase tracking-wider mb-1">Container ID</p>
                <p className="font-mono text-xs">{server.ContainerID}</p>
             </div>
             <div className="bg-white/5 p-4 rounded-2xl border border-white/5">
                <p className="text-[10px] text-gray-400 font-bold uppercase tracking-wider mb-1">Image</p>
                <p className="text-xs truncate">{server.Image}</p>
             </div>
             <div className="bg-white/5 p-4 rounded-2xl border border-white/5">
                <p className="text-[10px] text-gray-400 font-bold uppercase tracking-wider mb-1">Network</p>
                <p className="text-xs">Bridge</p>
             </div>
             <div className="bg-white/5 p-4 rounded-2xl border border-white/5">
                <p className="text-[10px] text-gray-400 font-bold uppercase tracking-wider mb-1">Ports</p>
                <p className="text-xs">{port}</p>
             </div>
          </div>
        </div>
      </Modal>
    </>
  );
};

export default ServerCard;
