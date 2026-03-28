import axios from 'axios';
import { getCached, setCached, invalidateCache } from './cache';
import { enqueueReferralPayload, flushQueuedReferrals, getQueuedReferralCount } from './offlineQueue';

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:3000/api';

// Cache TTL values in milliseconds
const CACHE_TTL = {
  directory: 5 * 60 * 1000,          // 5 minutes
  adminDepartments: 5 * 60 * 1000,   // 5 minutes
  adminStats: 60 * 1000,              // 1 minute
  analystStats: 60 * 1000,            // 1 minute
};

// Generate cache key
const generateCacheKey = (method, url, params = {}) => {
  const baseKey = `${method}:${url}`;
  if (Object.keys(params).length > 0) {
    return `${baseKey}:${JSON.stringify(params)}`;
  }
  return baseKey;
};

// Get cache TTL for URL
const getCacheTTL = (url) => {
  if (url.includes('/directory')) return CACHE_TTL.directory;
  if (url.includes('/admin/departments')) return CACHE_TTL.adminDepartments;
  if (url.includes('/admin/stats')) return CACHE_TTL.adminStats;
  if (url.includes('/analyst/stats')) return CACHE_TTL.analystStats;
  return null;
};

// Create an Axios instance with base URL
export const api = axios.create({
  baseURL: API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor - try to get from cache first for GET requests
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }

    // Check cache for GET requests
    if (config.method === 'get') {
      const cacheKey = generateCacheKey(config.method, config.url, config.params);
      const cachedData = getCached(cacheKey);
      if (cachedData) {
        // Return cached data by creating a fake response object
        // that mimics axios response structure
        config.adapter = (config) => {
          return Promise.resolve({
            data: cachedData,
            status: 200,
            statusText: 'OK',
            headers: new axios.AxiosHeaders(config.headers),
            config,
            request: {
              responseURL: config.url,
            },
          });
        };
      }
    }

    return config;
  },
  (error) => Promise.reject(error)
);

// Response interceptor to cache GET responses
api.interceptors.response.use(
  (response) => {
    const { config } = response;
    if (config.method === 'get') {
      const ttl = getCacheTTL(config.url);
      if (ttl) {
        const cacheKey = generateCacheKey(config.method, config.url, config.params);
        setCached(cacheKey, response.data, ttl);
      }
    }
    return response;
  },
  (error) => {
    if (error.response && error.response.status === 401) {
      // Clear storage and redirect to login if not already there
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      if (window.location.pathname !== '/login') {
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

// ──────────────────────────────────────────────────────────────────────
// Auth Services
// ──────────────────────────────────────────────────────────────────────

export const login = async (username, password) => {
  const { data } = await api.post('/login', { username, password });
  if (data.token) {
    localStorage.setItem('token', data.token);
    localStorage.setItem('user', JSON.stringify(data.user));
  }
  return data;
};

export const logout = () => {
  localStorage.removeItem('token');
  localStorage.removeItem('user');
  // Clear all cache on logout
  invalidateCache();
};

// ──────────────────────────────────────────────────────────────────────
// Directory & Common Services
// ──────────────────────────────────────────────────────────────────────

export const getDirectory = async () => {
  const { data } = await api.get('/directory');
  return data;
};

// ──────────────────────────────────────────────────────────────────────
// Level 2 Doctor Services
// ──────────────────────────────────────────────────────────────────────

export const checkNotifications = async () => {
  const { data } = await api.get('/notifications');
  return data;
};

export const markNotificationRead = async (id) => {
  const { data } = await api.patch(`/notifications/${id}/read`);
  return data;
};

export const suggestDepartment = async (payload) => {
  const { data } = await api.post('/referrals/suggest', payload);
  return data;
};

export const createReferral = async (payload) => {
  const { data } = await api.post('/referrals', payload);
  // Invalidate cache after creating referral
  invalidateCache('/analyst');
  return data;
};

export const queueReferralForSync = async (payload) => {
  return enqueueReferralPayload(payload);
};

let syncInProgress = false;

export const syncQueuedReferrals = async () => {
  if (syncInProgress) return { sent: 0, failed: 0 };
  if (!navigator.onLine) return { sent: 0, failed: 0 };

  syncInProgress = true;
  try {
    const result = await flushQueuedReferrals(async (payload) => {
      await createReferral(payload);
    });
    return result;
  } finally {
    syncInProgress = false;
  }
};

export const initStoreAndForwardSync = () => {
  window.addEventListener('online', () => {
    syncQueuedReferrals().catch((error) => {
      console.error('[OFFLINE SYNC] Online sync failed', error);
    });
  });

  if (navigator.onLine) {
    syncQueuedReferrals().catch((error) => {
      console.error('[OFFLINE SYNC] Initial sync failed', error);
    });
  }
};

export const getOfflineReferralQueueCount = async () => {
  return getQueuedReferralCount();
};

export const uploadAttachments = async (referralID, files) => {
  const formData = new FormData();
  for (let i = 0; i < files.length; i++) {
    formData.append('attachments', files[i]);
  }
  const { data } = await api.post(`/referrals/${referralID}/attachments`, formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
  return data;
};

// Get attachment URL with auth for viewing inline (images/PDFs)
// Returns an object with URL and headers for authenticated viewing
export const getAttachmentUrl = (id) => {
  const token = localStorage.getItem('token');
  return {
    url: `${API_URL}/attachments/${id}`,
    authHeader: `Bearer ${token}`
  };
};

// Legacy URL getter - only use for non-authenticated contexts
export const getAttachmentUrlLegacy = (id) => `${API_URL}/attachments/${id}`;

// Download attachment with authentication - returns blob for download
export const downloadAttachment = async (id) => {
  const token = localStorage.getItem('token');
  const response = await fetch(`${API_URL}/attachments/${id}?token=${encodeURIComponent(token)}`, {
    method: 'GET',
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });

  if (!response.ok) {
    throw new Error(`Failed to download attachment: ${response.statusText}`);
  }

  return response.blob();
};

// ──────────────────────────────────────────────────────────────────────
// CHU Doctor Services
// ──────────────────────────────────────────────────────────────────────

export const getQueue = async () => {
  const { data } = await api.get('/queue');
  return data;
};

export const getReferralDetails = async (id) => {
  const { data } = await api.get(`/referrals/${id}`);
  return data;
};

export const scheduleReferral = async (id, appointmentDate) => {
  const { data } = await api.patch(`/referrals/${id}/schedule`, { appointment_date: appointmentDate });
  // Invalidate cache after scheduling
  invalidateCache('/admin');
  invalidateCache('/analyst');
  return data;
};

export const redirectReferral = async (id, newDepartmentID, reason) => {
  const { data } = await api.patch(`/referrals/${id}/redirect`, {
    new_department_id: newDepartmentID,
    reason: reason,
  });
  // Invalidate cache after redirect
  invalidateCache('/admin');
  invalidateCache('/analyst');
  return data;
};

export const denyReferral = async (id, reason) => {
  const { data } = await api.patch(`/referrals/${id}/deny`, { reason });
  // Invalidate cache after denying
  invalidateCache('/admin');
  invalidateCache('/analyst');
  return data;
};

// ── Admin Endpoints ──
export const getAdminStats = async () => {
  const { data } = await api.get('/admin/stats');
  return data;
};

export const getUsers = async () => {
  const { data } = await api.get('/admin/users');
  // Handle both paginated { users: [], pagination: {} } and legacy array responses
  return Array.isArray(data) ? data : (data.users || []);
};

export const createUser = async (payload) => {
  const { data } = await api.post('/admin/users', payload);
  return data;
};

export const deleteUser = async (id) => {
  const { data } = await api.delete(`/admin/users/${id}`);
  return data;
};

export const getAdminDepartments = async () => {
  const { data } = await api.get('/admin/departments');
  // Handle both paginated { departments: [], pagination: {} } and legacy array responses
  return Array.isArray(data) ? data : (data.departments || []);
};

export const createDepartment = async (payload) => {
  const { data } = await api.post('/admin/departments', payload);
  // Invalidate directory and department cache
  invalidateCache('/directory');
  invalidateCache('/admin/departments');
  return data;
};

export const updateDepartment = async (id, payload) => {
  const { data } = await api.patch(`/admin/departments/${id}`, payload);
  // Invalidate directory and department cache
  invalidateCache('/directory');
  invalidateCache('/admin/departments');
  return data;
};

export const deleteDepartment = async (id) => {
  const { data } = await api.delete(`/admin/departments/${id}`);
  // Invalidate directory and department cache
  invalidateCache('/directory');
  invalidateCache('/admin/departments');
  return data;
};

// ── Analyst Endpoints ──
export const getAnalystStats = async () => {
  const { data } = await api.get('/analyst/stats/departments');
  // Handle paginated response format: {departments: [], pagination: {}}
  return data.departments || data || [];
};

export const getAnalystDoctorStats = async () => {
  const { data } = await api.get('/analyst/stats/doctors');
  // Handle paginated response format: {doctors: [], pagination: {}} or direct array
  return data.doctors || data || [];
};

// ── History & Rescheduling ──
export const getHistory = async () => {
  const { data } = await api.get('/history');
  return data;
};

export const rescheduleReferral = async (id, appointmentDate) => {
  const { data } = await api.patch(`/referrals/${id}/reschedule`, { appointment_date: appointmentDate });
  // Invalidate cache after rescheduling
  invalidateCache('/admin');
  invalidateCache('/analyst');
  return data;
};

export const cancelReferral = async (id) => {
  const { data } = await api.patch(`/referrals/${id}/cancel`);
  // Invalidate cache after cancelling
  invalidateCache('/admin');
  invalidateCache('/analyst');
  return data;
};

export default api;
