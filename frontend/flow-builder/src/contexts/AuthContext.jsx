import { createContext, useContext, useState } from 'react';

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  // Get username from environment variable or default to 'sdutt'
  const [username] = useState(() => {
    return import.meta.env.VITE_DEV_USERNAME || 'sdutt';
  });

  const value = {
    username,
    isAuthenticated: !!username,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
