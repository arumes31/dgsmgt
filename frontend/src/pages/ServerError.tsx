import React from 'react';
import { RefreshCcw, AlertTriangle } from 'lucide-react';
import { motion } from 'framer-motion';

const ServerError: React.FC = () => {
  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <motion.div 
        initial={{ opacity: 0, scale: 0.9 }}
        animate={{ opacity: 1, scale: 1 }}
        className="glass max-w-md w-full p-8 rounded-3xl text-center border border-white/10"
      >
        <div className="w-20 h-20 bg-red-500/10 rounded-2xl flex items-center justify-center mx-auto mb-6">
          <AlertTriangle size={40} className="text-red-500" />
        </div>
        <h1 className="text-6xl font-bold mb-2">500</h1>
        <h2 className="text-2xl font-semibold mb-4">Server Error</h2>
        <p className="text-gray-500 mb-8">
          Something went wrong on our end. We're working on fixing it.
        </p>
        <button
          onClick={() => window.location.reload()}
          className="w-full flex items-center justify-center gap-2 bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-medium transition-all shadow-lg shadow-red-500/25"
        >
          <RefreshCcw size={18} />
          Try Again
        </button>
      </motion.div>
    </div>
  );
};

export default ServerError;
