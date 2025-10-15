import { useState, useEffect } from 'react';
import {
  FiCode,
  FiGlobe,
  FiGitBranch,
  FiRepeat,
  FiLayers,
  FiFilter,
  FiZap,
  FiPackage,
  FiSearch,
  FiUser
} from 'react-icons/fi';
import { MdSmartToy, MdHttp } from 'react-icons/md';

/**
 * Hook to load node registry from JSON
 * Fetches node definitions, configs, and status from backend-controlled registry
 */
export function useNodeRegistry() {
  const [registry, setRegistry] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const loadRegistry = async () => {
      try {
        const response = await fetch('/node-registry.json');
        if (!response.ok) {
          throw new Error('Failed to load node registry');
        }
        const data = await response.json();
        setRegistry(data);
      } catch (err) {
        console.error('Failed to load node registry:', err);
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    loadRegistry();
  }, []);

  return {
    registry,
    loading,
    error,
    // Helper functions
    getNode: (nodeType) => registry?.nodes?.[nodeType],
    getActiveNodes: () => {
      if (!registry?.nodes) return [];
      return Object.values(registry.nodes).filter(n => n.status === 'active');
    },
    getNodesByCategory: (category) => {
      if (!registry?.nodes) return [];
      return Object.values(registry.nodes).filter(n => n.category === category);
    },
    getNodesByStatus: (status) => {
      if (!registry?.nodes) return [];
      return Object.values(registry.nodes).filter(n => n.status === status);
    }
  };
}

/**
 * Get node icon component from icon name string
 * Maps icon names from JSON to actual React icon components
 */
export function getIconComponent(iconName) {
  const iconMap = {
    'MdSmartToy': MdSmartToy,
    'MdHttp': MdHttp,
    'FiCode': FiCode,
    'FiGlobe': FiGlobe,
    'FiGitBranch': FiGitBranch,
    'FiRepeat': FiRepeat,
    'FiLayers': FiLayers,
    'FiFilter': FiFilter,
    'FiZap': FiZap,
    'FiPackage': FiPackage,
    'FiSearch': FiSearch,
    'FiUser': FiUser,
  };

  return iconMap[iconName] || FiCode;
}
