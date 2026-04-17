import React from 'react';
import { NavLink } from 'react-router-dom';
import { LayoutDashboard, Users, Server, ShieldCheck, LogOut } from 'lucide-react';

interface SidebarProps {
  isAdmin: boolean;
}

const Sidebar: React.FC<SidebarProps> = ({ isAdmin }) => {
  const handleLogout = () => {
    localStorage.removeItem('token');
    window.location.href = '/login';
  };

  const navItems = [
    { name: 'Dashboard', icon: LayoutDashboard, path: '/dashboard' },
  ];

  const adminItems = [
    { name: 'Users', icon: Users, path: '/admin/users' },
    { name: 'Servers', icon: Server, path: '/admin/servers' },
    { name: 'Assignments', icon: ShieldCheck, path: '/admin/assignments' },
  ];

  return (
    <div className="w-64 min-h-screen glass border-r border-glass-border p-6 flex flex-col">
      <div className="mb-8">
        <h1 className="text-2xl font-bold bg-gradient-to-r from-blue-500 to-purple-600 bg-clip-text text-transparent">
          DGS MGT
        </h1>
      </div>

      <nav className="flex-1 space-y-2">
        {navItems.map((item) => (
          <NavLink
            key={item.path}
            to={item.path}
            className={({ isActive }) =>
              `flex items-center space-x-3 p-3 rounded-xl transition-all duration-300 ${
                isActive
                  ? 'bg-blue-500/10 shadow-lg text-blue-500'
                  : 'opacity-60 hover:bg-foreground/5 hover:opacity-100'
              }`
            }
          >
            <item.icon size={20} />
            <span className="font-medium">{item.name}</span>
          </NavLink>
        ))}

        {isAdmin && (
          <>
            <div className="pt-6 pb-2 px-3">
              <span className="text-xs font-semibold opacity-40 uppercase tracking-wider">
                Admin Area
              </span>
            </div>
            {adminItems.map((item) => (
              <NavLink
                key={item.path}
                to={item.path}
                className={({ isActive }) =>
                  `flex items-center space-x-3 p-3 rounded-xl transition-all duration-300 ${
                    isActive
                      ? 'bg-purple-500/10 shadow-lg text-purple-500'
                      : 'opacity-60 hover:bg-foreground/5 hover:opacity-100'
                  }`
                }
              >
                <item.icon size={20} />
                <span className="font-medium">{item.name}</span>
              </NavLink>
            ))}
          </>
        )}
      </nav>

      <button
        onClick={handleLogout}
        className="mt-auto flex items-center space-x-3 p-3 rounded-xl opacity-60 hover:bg-red-500/10 hover:text-red-500 hover:opacity-100 transition-all duration-300 cursor-pointer"
      >
        <LogOut size={20} />
        <span className="font-medium">Logout</span>
      </button>
    </div>
  );
};

export default Sidebar;
