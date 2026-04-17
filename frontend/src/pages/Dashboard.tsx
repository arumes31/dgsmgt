import React, { useEffect, useState } from 'react';
import { motion } from 'framer-motion';
import { Server as ServerIcon, Activity } from 'lucide-react';
import { getMyServers } from '../api';
import type { ServerWithPerms } from '../api';
import ServerCard from '../components/ServerCard';
import Skeleton from '../components/Skeleton';

const Dashboard: React.FC = () => {
  const [servers, setServers] = useState<ServerWithPerms[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchServers();
    const interval = setInterval(fetchServers, 10000); // Poll every 10 seconds
    return () => clearInterval(interval);
  }, []);

  const fetchServers = async () => {
    try {
      const data = await getMyServers();
      setServers(data);
      setError(null);
    } catch (err) {
      console.error('Failed to fetch servers:', err);
      setError('Failed to load servers. Please try again later.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold mb-2">Dashboard</h1>
          <p className="opacity-60">Manage your game server instances and monitor their status.</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="glass px-4 py-2 rounded-xl flex items-center gap-2 text-sm opacity-80 border border-white/5">
            <Activity size={16} className="text-green-500" />
            System Online
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div className="glass rounded-2xl p-6 border border-white/5">
          <p className="opacity-60 text-sm font-medium mb-1">Total Servers</p>
          {loading && servers.length === 0 ? (
            <Skeleton className="h-9 w-12" />
          ) : (
            <p className="text-3xl font-bold text-blue-500">{servers.length}</p>
          )}
        </div>
        <div className="glass rounded-2xl p-6 border border-white/5">
          <p className="opacity-60 text-sm font-medium mb-1">Active Services</p>
          {loading && servers.length === 0 ? (
            <Skeleton className="h-9 w-12" />
          ) : (
            <p className="text-3xl font-bold text-green-500">
              {servers.filter(s => s.ContainerID).length}
            </p>
          )}
        </div>
        <div className="glass rounded-2xl p-6 border border-white/5">
          <p className="opacity-60 text-sm font-medium mb-1">System Load</p>
          <p className="text-3xl font-bold text-purple-500">Minimal</p>
        </div>
      </div>

      {loading && servers.length === 0 ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-6">
          {[1, 2, 3].map((i) => (
            <div key={i} className="glass rounded-3xl p-6 h-64 border border-white/5 flex flex-col gap-4">
              <div className="flex justify-between items-start">
                <div className="flex items-center gap-3">
                  <Skeleton className="w-12 h-12 rounded-2xl" />
                  <div className="space-y-2">
                    <Skeleton className="h-5 w-32" />
                    <Skeleton className="h-3 w-20" />
                  </div>
                </div>
                <Skeleton className="w-20 h-8 rounded-full" />
              </div>
              <Skeleton className="flex-1 w-full rounded-2xl" />
              <div className="flex gap-2">
                <Skeleton className="h-10 flex-1 rounded-xl" />
                <Skeleton className="h-10 flex-1 rounded-xl" />
              </div>
            </div>
          ))}
        </div>
      ) : error ? (
        <div className="glass rounded-3xl p-12 flex flex-col items-center justify-center text-center border border-red-500/10">
          <div className="w-16 h-16 bg-red-500/10 rounded-full flex items-center justify-center mb-6">
            <Activity size={32} className="text-red-500" />
          </div>
          <h3 className="text-xl font-medium mb-2">Error Loading Data</h3>
          <p className="opacity-50 max-w-md">{error}</p>
        </div>
      ) : servers.length === 0 ? (
        <div className="glass rounded-3xl p-12 flex flex-col items-center justify-center text-center border border-dashed border-white/10">
          <div className="w-16 h-16 bg-white/5 rounded-full flex items-center justify-center mb-6">
            <ServerIcon size={32} className="opacity-40" />
          </div>
          <h3 className="text-xl font-medium mb-2">No Servers Assigned</h3>
          <p className="opacity-50 max-w-md">
            You don't have any game servers assigned to your account yet. 
            Please contact an administrator to allocate resources to you.
          </p>
        </div>
      ) : (
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-6"
        >
          {servers.map((server) => (
            <ServerCard key={server.ID} server={server} onAction={fetchServers} />
          ))}
        </motion.div>
      )}
    </div>
  );
};

export default Dashboard;
