import React, { useEffect, useRef, useState } from 'react';
import {
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  AreaChart,
  Area,
} from 'recharts';

interface MetricsChartProps {
  serverId: string;
}

interface MetricData {
  time: string;
  cpu: number;
  ram: number;
}

const MetricsChart: React.FC<MetricsChartProps> = ({ serverId }) => {
  const [data, setData] = useState<MetricData[]>([]);
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = import.meta.env.VITE_API_URL 
      ? import.meta.env.VITE_API_URL.replace(/^https?:\/\//, '') 
      : window.location.host;
    const token = localStorage.getItem('token');
    
    const wsUrl = `${protocol}//${host}/api/metrics/${serverId}?token=${token}`;
    const socket = new WebSocket(wsUrl);
    socketRef.current = socket;

    socket.onmessage = (event) => {
      try {
        const stats = JSON.parse(event.data);
        
        // Calculate CPU percentage
        let cpuPercent = 0;
        if (stats.cpu_stats && stats.precpu_stats) {
          const cpuDelta = stats.cpu_stats.cpu_usage.total_usage - stats.precpu_stats.cpu_usage.total_usage;
          const systemDelta = stats.cpu_stats.system_cpu_usage - stats.precpu_stats.system_cpu_usage;
          const onlineCpus = stats.cpu_stats.online_cpus || 1;
          
          if (systemDelta > 0 && cpuDelta > 0) {
            cpuPercent = (cpuDelta / systemDelta) * onlineCpus * 100.0;
          }
        }

        // Calculate RAM percentage
        let ramPercent = 0;
        if (stats.memory_stats && stats.memory_stats.usage && stats.memory_stats.limit) {
          ramPercent = (stats.memory_stats.usage / stats.memory_stats.limit) * 100.0;
        }

        const newData: MetricData = {
          time: new Date().toLocaleTimeString(),
          cpu: parseFloat(cpuPercent.toFixed(2)),
          ram: parseFloat(ramPercent.toFixed(2)),
        };

        setData((prev) => {
          const updated = [...prev, newData];
          if (updated.length > 20) {
            return updated.slice(updated.length - 20);
          }
          return updated;
        });
      } catch (err) {
        console.error('Error parsing metrics:', err);
      }
    };

    return () => {
      socket.close();
    };
  }, [serverId]);

  return (
    <div className="glass rounded-xl p-4 border border-glass-border h-[300px] w-full flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-400">System Metrics</h3>
        <div className="flex gap-4 text-xs">
          <div className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-blue-500"></span>
            <span>CPU {data.length > 0 ? data[data.length - 1].cpu : 0}%</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-purple-500"></span>
            <span>RAM {data.length > 0 ? data[data.length - 1].ram : 0}%</span>
          </div>
        </div>
      </div>
      
      <div className="flex-1 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data}>
            <defs>
              <linearGradient id="colorCpu" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/>
                <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
              </linearGradient>
              <linearGradient id="colorRam" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#a855f7" stopOpacity={0.3}/>
                <stop offset="95%" stopColor="#a855f7" stopOpacity={0}/>
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" vertical={false} />
            <XAxis 
              dataKey="time" 
              hide={true}
            />
            <YAxis 
              domain={[0, 100]} 
              tick={{ fontSize: 10, fill: '#94a3b8' }}
              axisLine={false}
              tickLine={false}
              tickFormatter={(val) => `${val}%`}
            />
            <Tooltip 
              contentStyle={{ 
                backgroundColor: 'rgba(15, 23, 42, 0.9)', 
                border: '1px solid rgba(255,255,255,0.1)',
                borderRadius: '8px',
                fontSize: '12px'
              }}
              itemStyle={{ padding: '0' }}
            />
            <Area 
              type="monotone" 
              dataKey="cpu" 
              stroke="#3b82f6" 
              fillOpacity={1} 
              fill="url(#colorCpu)" 
              strokeWidth={2}
              isAnimationActive={false}
            />
            <Area 
              type="monotone" 
              dataKey="ram" 
              stroke="#a855f7" 
              fillOpacity={1} 
              fill="url(#colorRam)" 
              strokeWidth={2}
              isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};

export default MetricsChart;
