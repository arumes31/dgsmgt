import axios from 'axios';

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '', // Relative if not specified
});

// Add a request interceptor to include JWT token
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Response interceptor to handle 401 Unauthorized
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      localStorage.removeItem('token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

export const login = async (username: string, password: string): Promise<{ token: string }> => {
  const response = await api.post('/api/login', { username, password });
  return response.data;
};

export interface User {
  ID: number;
  Username: string;
  IsAdmin: boolean;
}

export interface Server {
  ID: number;
  Name: string;
  ContainerID: string;
  Image: string;
  ConfigJSON: string;
}

export interface ServerWithPerms extends Server {
  can_start: boolean;
  can_stop: boolean;
  can_restart: boolean;
  can_view_logs: boolean;
}

export const getMyServers = async (): Promise<ServerWithPerms[]> => {
  const response = await api.get('/api/my-servers');
  return response.data;
};

export const getUsers = async (): Promise<User[]> => {
  const response = await api.get('/api/admin/users');
  return response.data;
};

export const createUser = async (user: any): Promise<User> => {
  const response = await api.post('/api/admin/users', user);
  return response.data;
};

export const updateUser = async (id: number, user: any): Promise<User> => {
  const response = await api.put(`/api/admin/users/${id}`, user);
  return response.data;
};

export const deleteUser = async (id: number): Promise<void> => {
  await api.delete(`/api/admin/users/${id}`);
};

export const getServers = async (): Promise<Server[]> => {
  const response = await api.get('/api/admin/servers');
  return response.data;
};

export const createServer = async (server: any): Promise<Server> => {
  const response = await api.post('/api/admin/servers', server);
  return response.data;
};

export const deleteServer = async (id: number): Promise<void> => {
  await api.delete(`/api/admin/servers/${id}`);
};

export const assignServer = async (assignment: any): Promise<void> => {
  await api.post('/api/admin/assign', assignment);
};

export default api;
