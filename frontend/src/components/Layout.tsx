import React from 'react';
import Sidebar from './Sidebar';
import ThemeToggle from './ThemeToggle';
import { getDecodedToken } from '../utils/auth';

interface LayoutProps {
  children: React.ReactNode;
}

const Layout: React.FC<LayoutProps> = ({ children }) => {
  const decodedToken = getDecodedToken();
  const isAdmin = decodedToken?.is_admin || false;
  const username = decodedToken?.username || 'User';

  return (
    <div className="flex min-h-screen bg-background text-foreground">
      <Sidebar isAdmin={isAdmin} />
      <div className="flex-1 flex flex-col h-screen overflow-hidden">
        <header className="h-20 glass border-b border-glass-border flex items-center justify-between px-8 z-10">
          <h2 className="text-xl font-semibold bg-gradient-to-r from-blue-500 to-purple-500 bg-clip-text text-transparent">
            Central Management
          </h2>
          <div className="flex items-center space-x-6">
            <ThemeToggle />
            <div className="flex items-center space-x-4">
              <span className="text-sm font-medium opacity-60">Welcome, {username}</span>
              <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center font-bold text-white shadow-lg shadow-blue-500/20">
                {username.substring(0, 2).toUpperCase()}
              </div>
            </div>
          </div>
        </header>
        <main className="flex-1 overflow-y-auto p-8 custom-scrollbar">
          {children}
        </main>
      </div>
    </div>
  );
};

export default Layout;
