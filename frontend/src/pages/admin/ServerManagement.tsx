import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Plus, Trash2, X, Server as ServerIcon, Cpu, Globe, HardDrive, List } from 'lucide-react';
import { getServers, createServer, deleteServer } from '../../api';
import type { Server } from '../../api';

const ServerManagement: React.FC = () => {
  const [servers, setServers] = useState<Server[]>([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [formData, setFormData] = useState({
    name: '',
    image: '',
    ports: '',
    env: '',
    volumes: '',
  });

  const fetchServers = async () => {
    try {
      const data = await getServers();
      setServers(data);
    } catch (error) {
      console.error('Failed to fetch servers:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchServers();
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const payload = {
        name: formData.name,
        image: formData.image,
        ports: formData.ports.split(',').map(s => s.trim()).filter(s => s !== ''),
        env: formData.env.split(',').map(s => s.trim()).filter(s => s !== ''),
        volumes: formData.volumes.split(',').map(s => s.trim()).filter(s => s !== ''),
      };
      await createServer(payload);
      setIsModalOpen(false);
      setFormData({ name: '', image: '', ports: '', env: '', volumes: '' });
      fetchServers();
    } catch (error) {
      console.error('Failed to create server:', error);
    }
  };

  const handleDelete = async (id: number) => {
    if (window.confirm('Are you sure you want to delete this server? This will also remove the container.')) {
      try {
        await deleteServer(id);
        fetchServers();
      } catch (error) {
        console.error('Failed to delete server:', error);
      }
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-3xl font-bold text-white">Server Management</h2>
          <p className="text-gray-400">Deploy and manage Docker containers</p>
        </div>
        <button
          onClick={() => setIsModalOpen(true)}
          className="flex items-center space-x-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-xl transition-colors shadow-lg"
        >
          <Plus size={20} />
          <span>New Server</span>
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {loading ? (
          <p className="text-gray-400">Loading...</p>
        ) : servers.length === 0 ? (
          <p className="text-gray-400">No servers found</p>
        ) : (
          servers.map((server) => (
            <motion.div
              key={server.ID}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              className="glass p-6 rounded-2xl border border-white/20 space-y-4 hover:shadow-xl transition-all group"
            >
              <div className="flex justify-between items-start">
                <div className="flex items-center space-x-3">
                  <div className="w-12 h-12 rounded-xl bg-blue-500/20 flex items-center justify-center text-blue-400">
                    <ServerIcon size={24} />
                  </div>
                  <div>
                    <h3 className="text-xl font-bold text-white">{server.Name}</h3>
                    <p className="text-xs text-gray-400 font-mono truncate w-40">
                      {server.ContainerID}
                    </p>
                  </div>
                </div>
                <button
                  onClick={() => handleDelete(server.ID)}
                  className="p-2 text-gray-500 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-all"
                >
                  <Trash2 size={18} />
                </button>
              </div>

              <div className="space-y-2">
                <div className="flex items-center space-x-2 text-sm text-gray-300">
                  <Cpu size={14} className="text-blue-400" />
                  <span className="truncate">{server.Image}</span>
                </div>
              </div>

              <div className="pt-4 border-t border-white/10 flex justify-between items-center">
                <span className="text-xs font-medium text-gray-500 uppercase">Configuration</span>
                <div className="flex space-x-2">
                  <Globe size={14} className="text-gray-400" />
                  <HardDrive size={14} className="text-gray-400" />
                  <List size={14} className="text-gray-400" />
                </div>
              </div>
            </motion.div>
          ))
        )}
      </div>

      <AnimatePresence>
        {isModalOpen && (
          <div className="fixed inset-0 flex items-center justify-center z-50 px-4">
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              onClick={() => setIsModalOpen(false)}
              className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            />
            <motion.div
              initial={{ scale: 0.95, opacity: 0, y: 20 }}
              animate={{ scale: 1, opacity: 1, y: 0 }}
              exit={{ scale: 0.95, opacity: 0, y: 20 }}
              className="relative w-full max-w-2xl glass-card p-8 rounded-3xl border border-white/20 shadow-2xl"
            >
              <div className="flex justify-between items-center mb-6">
                <h3 className="text-2xl font-bold text-white">Create New Server</h3>
                <button
                  onClick={() => setIsModalOpen(false)}
                  className="p-2 text-gray-400 hover:text-white transition-colors"
                >
                  <X size={24} />
                </button>
              </div>

              <form onSubmit={handleSubmit} className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">Server Name</label>
                    <input
                      type="text"
                      required
                      value={formData.name}
                      onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                      className="w-full px-4 py-3 bg-white/5 border border-white/10 rounded-xl text-white focus:outline-none focus:ring-2 focus:ring-blue-500 transition-all"
                      placeholder="My Minecraft Server"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">Docker Image</label>
                    <input
                      type="text"
                      required
                      value={formData.image}
                      onChange={(e) => setFormData({ ...formData, image: e.target.value })}
                      className="w-full px-4 py-3 bg-white/5 border border-white/10 rounded-xl text-white focus:outline-none focus:ring-2 focus:ring-blue-500 transition-all"
                      placeholder="itzg/minecraft-server"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">Ports (comma separated)</label>
                    <input
                      type="text"
                      value={formData.ports}
                      onChange={(e) => setFormData({ ...formData, ports: e.target.value })}
                      className="w-full px-4 py-3 bg-white/5 border border-white/10 rounded-xl text-white focus:outline-none focus:ring-2 focus:ring-blue-500 transition-all"
                      placeholder="25565:25565"
                    />
                  </div>
                </div>
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">Environment (comma separated)</label>
                    <textarea
                      value={formData.env}
                      onChange={(e) => setFormData({ ...formData, env: e.target.value })}
                      className="w-full px-4 py-3 bg-white/5 border border-white/10 rounded-xl text-white focus:outline-none focus:ring-2 focus:ring-blue-500 transition-all h-32"
                      placeholder="EULA=TRUE, ONLINE_MODE=FALSE"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">Volumes (comma separated)</label>
                    <input
                      type="text"
                      value={formData.volumes}
                      onChange={(e) => setFormData({ ...formData, volumes: e.target.value })}
                      className="w-full px-4 py-3 bg-white/5 border border-white/10 rounded-xl text-white focus:outline-none focus:ring-2 focus:ring-blue-500 transition-all"
                      placeholder="/opt/mcdata:/data"
                    />
                  </div>
                </div>

                <div className="md:col-span-2 pt-4">
                  <button
                    type="submit"
                    className="w-full py-4 bg-gradient-to-r from-blue-600 to-purple-600 text-white font-bold rounded-xl hover:shadow-lg hover:shadow-blue-500/20 transition-all flex items-center justify-center space-x-2"
                  >
                    <Plus size={20} />
                    <span>Deploy Server</span>
                  </button>
                </div>
              </form>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
    </div>
  );
};

export default ServerManagement;
