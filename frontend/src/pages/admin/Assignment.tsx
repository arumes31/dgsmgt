import React, { useState, useEffect } from 'react';
import { motion } from 'framer-motion';
import { ShieldCheck, User as UserIcon, Server as ServerIcon, CheckCircle2, Circle } from 'lucide-react';
import { getUsers, getServers, assignServer } from '../../api';
import type { User, Server } from '../../api';

const Assignment: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [servers, setServers] = useState<Server[]>([]);
  const [selectedUser, setSelectedUser] = useState<number | null>(null);
  const [selectedServer, setSelectedServer] = useState<number | null>(null);
  const [permissions, setPermissions] = useState({
    can_start: true,
    can_stop: true,
    can_restart: true,
    can_view_logs: true,
  });
  const [loading, setLoading] = useState(true);
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [userData, serverData] = await Promise.all([getUsers(), getServers()]);
        setUsers(userData);
        setServers(serverData);
      } catch (error) {
        console.error('Failed to fetch data:', error);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, []);

  const handleAssign = async () => {
    if (!selectedUser || !selectedServer) return;

    try {
      await assignServer({
        user_id: selectedUser,
        server_id: selectedServer,
        ...permissions,
      });
      setSuccess(true);
      setTimeout(() => setSuccess(false), 3000);
    } catch (error) {
      console.error('Failed to assign permissions:', error);
    }
  };

  const togglePermission = (key: keyof typeof permissions) => {
    setPermissions(prev => ({ ...prev, [key]: !prev[key] }));
  };

  return (
    <div className="max-w-4xl mx-auto space-y-8">
      <div>
        <h2 className="text-3xl font-bold text-white">Permissions Assignment</h2>
        <p className="text-gray-400">Grant users access and control over specific servers</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
        {/* User Selection */}
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-300 flex items-center space-x-2">
            <UserIcon size={18} className="text-blue-400" />
            <span>Select User</span>
          </h3>
          <div className="glass rounded-2xl border border-white/10 overflow-hidden max-h-64 overflow-y-auto">
            {loading ? (
              <p className="p-4 text-gray-500">Loading users...</p>
            ) : (
              users.map(user => (
                <button
                  key={user.ID}
                  onClick={() => setSelectedUser(user.ID)}
                  className={`w-full p-4 text-left flex items-center justify-between transition-all ${
                    selectedUser === user.ID ? 'bg-blue-600/20 text-white' : 'text-gray-400 hover:bg-white/5'
                  }`}
                >
                  <span className="font-medium">{user.Username}</span>
                  {selectedUser === user.ID && <CheckCircle2 size={18} className="text-blue-400" />}
                </button>
              ))
            )}
          </div>
        </div>

        {/* Server Selection */}
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-300 flex items-center space-x-2">
            <ServerIcon size={18} className="text-purple-400" />
            <span>Select Server</span>
          </h3>
          <div className="glass rounded-2xl border border-white/10 overflow-hidden max-h-64 overflow-y-auto">
            {loading ? (
              <p className="p-4 text-gray-500">Loading servers...</p>
            ) : (
              servers.map(server => (
                <button
                  key={server.ID}
                  onClick={() => setSelectedServer(server.ID)}
                  className={`w-full p-4 text-left flex items-center justify-between transition-all ${
                    selectedServer === server.ID ? 'bg-purple-600/20 text-white' : 'text-gray-400 hover:bg-white/5'
                  }`}
                >
                  <span className="font-medium">{server.Name}</span>
                  {selectedServer === server.ID && <CheckCircle2 size={18} className="text-purple-400" />}
                </button>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Permissions Control */}
      <motion.div
        initial={false}
        animate={{ opacity: (selectedUser && selectedServer) ? 1 : 0.5 }}
        className="glass p-8 rounded-3xl border border-white/20 space-y-6"
      >
        <div className="flex items-center space-x-3 mb-2">
          <ShieldCheck size={24} className="text-green-400" />
          <h3 className="text-xl font-bold text-white">Define Permissions</h3>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {[
            { key: 'can_start', label: 'Can Start Server', color: 'blue' },
            { key: 'can_stop', label: 'Can Stop Server', color: 'red' },
            { key: 'can_restart', label: 'Can Restart Server', color: 'yellow' },
            { key: 'can_view_logs', label: 'Can View Logs', color: 'green' },
          ].map(perm => (
            <button
              key={perm.key}
              disabled={!selectedUser || !selectedServer}
              onClick={() => togglePermission(perm.key as keyof typeof permissions)}
              className={`flex items-center justify-between p-4 rounded-xl border transition-all ${
                permissions[perm.key as keyof typeof permissions]
                  ? 'bg-white/10 border-white/20 text-white'
                  : 'bg-transparent border-white/5 text-gray-500'
              }`}
            >
              <span className="font-medium">{perm.label}</span>
              {permissions[perm.key as keyof typeof permissions] ? (
                <CheckCircle2 size={20} className="text-green-400" />
              ) : (
                <Circle size={20} />
              )}
            </button>
          ))}
        </div>

        <div className="pt-4">
          <button
            disabled={!selectedUser || !selectedServer}
            onClick={handleAssign}
            className={`w-full py-4 rounded-2xl font-bold text-lg transition-all flex items-center justify-center space-x-2 ${
              success
                ? 'bg-green-600 text-white'
                : 'bg-gradient-to-r from-blue-600 to-purple-600 text-white hover:shadow-xl'
            }`}
          >
            {success ? (
              <>
                <CheckCircle2 size={22} />
                <span>Permissions Assigned Successfully!</span>
              </>
            ) : (
              <span>Confirm Assignment</span>
            )}
          </button>
        </div>
      </motion.div>
    </div>
  );
};

export default Assignment;
