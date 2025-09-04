import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  AppBar,
  Toolbar,
  Typography,
  Container,
  Box,
  Paper,
  TextField,
  Button,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Stack,
  Chip,
  IconButton,
  Snackbar,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Tabs,
  Tab,
  Divider,
  Tooltip,
  Table,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
  Switch,
  FormControlLabel,
  Checkbox,
} from "@mui/material";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import Grid from "@mui/material/Grid";
import LogoutIcon from "@mui/icons-material/Logout";
import UploadFileIcon from "@mui/icons-material/UploadFile";
import FileDownloadIcon from "@mui/icons-material/FileDownload";
import ManageAccountsIcon from "@mui/icons-material/ManageAccounts";
import IntegrationInstructionsIcon from "@mui/icons-material/IntegrationInstructions";
import AssessmentIcon from "@mui/icons-material/Assessment";
import SecurityIcon from "@mui/icons-material/Security";
import StorageIcon from "@mui/icons-material/Storage";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";
import FlagIcon from "@mui/icons-material/Flag";
import PolicyIcon from "@mui/icons-material/Policy";
import VerifiedUserIcon from "@mui/icons-material/VerifiedUser";
import SettingsEthernetIcon from "@mui/icons-material/SettingsEthernet";
import FactCheckIcon from "@mui/icons-material/FactCheck";
import GavelIcon from "@mui/icons-material/Gavel";

const API_BASE = process.env.REACT_APP_API_BASE || "http://localhost:8080/api";

const ENABLE_SSO_PROVIDERS = false;
const ENABLE_API_KEYS = false;


/* -------------------- Types -------------------- */
export type Exam = {
  id: string;
  title: string;
  time_limit_sec?: number;
  questions: Question[];
  profile?: string;
};

export type Question = {
  id: string;
  type: "mcq_single" | "mcq_multi" | "true_false" | "short_word" | "numeric" | "essay";
  prompt_html?: string;
  prompt?: string;
  choices?: { id: string; label_html?: string }[];
  points?: number;
};

export type ExamSummary = {
  id: string;
  title: string;
  time_limit_sec?: number;
  created_at?: number;
  profile?: string;
  status?: "pending" | "approved" | "archived";
};

export type Attempt = {
  id: string;
  exam_id: string;
  user_id: string;
  status: string;
  score?: number;
  responses?: Record<string, any>;
};

type User = {
  id: string;
  username: string;
  role: string;
};

type Features = { mode: "online"|"offline"; enable_google_auth: boolean };

/** NEW: Tenants & Flags */
type Tenant = { id: string; name: string; domain?: string; flags?: Record<string, boolean> };

export type Course = { id: string; name: string };

/* -------------------- Helpers -------------------- */
async function api<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, opts);
  if (!res.ok) {
    const t = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${t}`);
  }
  try { return await res.json(); } catch { return undefined as unknown as T; }
}

function htmlify(s?: string) { return { __html: s || "" }; }
function ms2date(ms?: number) { if (!ms) return ""; const d = new Date(ms * 1000); return d.toLocaleString(); }

function useSnack() {
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  return {
    msg, err, setMsg, setErr,
    node: (
      <>
        <Snackbar open={!!msg} autoHideDuration={3000} onClose={() => setMsg(null)} anchorOrigin={{ vertical: "bottom", horizontal: "center" }}>
          <Alert severity="success" variant="filled" onClose={() => setMsg(null)}>{msg}</Alert>
        </Snackbar>
        <Snackbar open={!!err} autoHideDuration={4500} onClose={() => setErr(null)} anchorOrigin={{ vertical: "bottom", horizontal: "center" }}>
          <Alert severity="error" variant="filled" onClose={() => setErr(null)}>{err}</Alert>
        </Snackbar>
      </>
    ),
  } as const;
}

function Shell({ children, authed, onSignOut, title, right }: { children: React.ReactNode; authed: boolean; onSignOut?: () => void; title?: string; right?: React.ReactNode; }) {
  return (
    <>
      <AppBar position="sticky" color="inherit" elevation={1} sx={{ backdropFilter: "saturate(180%) blur(6px)", background: "rgba(255,255,255,0.9)" }}>
        <Toolbar>
          <Box sx={{ width: 10, height: 10, bgcolor: "primary.main", borderRadius: 2, mr: 1.5 }} />
          <Typography variant="h6" sx={{ fontWeight: 600 }}>MindEngage • Admin</Typography>
          {title && (<Typography variant="body2" color="text.secondary" sx={{ ml: 2, display: { xs: "none", md: "block" } }}>{title}</Typography>)}
          <Box sx={{ flexGrow: 1 }} />
          {right}
          {authed && onSignOut && (
            <Tooltip title="Sign out"><IconButton onClick={onSignOut} color="primary" aria-label="sign out"><LogoutIcon /></IconButton></Tooltip>
          )}
        </Toolbar>
      </AppBar>
      <Container maxWidth="lg" sx={{ py: 4 }}>{children}</Container>
    </>
  );
}

/* -------------------- Login -------------------- */
function LoginScreen({ busy, onLogin, features }: { busy: boolean; onLogin: (u: string, p: string) => void; features?: { enable_google_auth: boolean } | null;}) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("admin");
  function submit(e: React.FormEvent) { e.preventDefault(); onLogin(username, password); }

  function googleSignIn() { window.location.href = `${API_BASE}/auth/google/login`; }
  const canGoogle = !!features?.enable_google_auth;

  return (
    <Shell authed={false} title="Sign in">
      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 7, lg: 6 }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="h5" fontWeight={600}>Admin Sign in</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>Use test creds (username=password). Role is enforced server-side by JWT.</Typography>
            <Box component="form" onSubmit={submit} sx={{ mt: 2 }}>
              <Stack spacing={2}>
                <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} fullWidth />
                <TextField label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} fullWidth />
                <Button type="submit" variant="contained" size="large" disableElevation disabled={busy}>{busy ? "…" : "Login"}</Button>
                {canGoogle && (
                  <Divider>or</Divider>
                )}
                {canGoogle && (
                  <Button variant="outlined" size="large" onClick={googleSignIn} disabled={busy}>Sign in with Google</Button>
                )}
              </Stack>
            </Box>
          </Paper>
        </Grid>
        <Grid size={{ xs: 12, md: 5, lg: 6 }}>
          <Paper elevation={0} sx={{ p: 3, height: "100%" }}>
            <Typography variant="subtitle1" fontWeight={600}>Admin responsibilities</Typography>
            <Box component="ul" sx={{ mt: 1.5, pl: 3 }}>
              <li>Identity & Role management</li>
              <li>Content governance & policy templates</li>
              <li>System health, integrations & security</li>
              <li>Compliance & audit</li>
            </Box>
          </Paper>
        </Grid>
      </Grid>
    </Shell>
  );
}

/* -------------------- Overview & Health (kept) -------------------- */
function OverviewPanel({ jwt }: { jwt: string; }) {
  const [health, setHealth] = useState<"unknown" | "ok" | "down">("unknown");
  const [ready, setReady] = useState<"unknown" | "ok" | "down">("unknown");
  const [cors, setCors] = useState<string>("(probe)");
  const snack = useSnack();

  async function probe(path: string): Promise<"ok" | "down"> {
    try { const res = await fetch(`${API_BASE}${path}`); return res.ok ? "ok" : "down"; } catch { return "down"; }
  }
  useEffect(() => {
    (async () => {
      setHealth(await probe("/healthz"));
      setReady(await probe("/readyz"));
      try {
        const res = await fetch(`${API_BASE}/exams` , { headers: { Authorization: `Bearer ${jwt}` } });
        setCors(res.ok ? "ok" : `blocked (${res.status})`);
      } catch { setCors("blocked"); }
    })();
  }, [jwt]);

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>System status</Typography>
        <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} sx={{ mt: 1.5 }}>
          <StatusCard label="/healthz" value={health} icon={<SecurityIcon color={health === 'ok' ? 'success' : 'error'} />} />
          <StatusCard label="/readyz" value={ready} icon={<StorageIcon color={ready === 'ok' ? 'success' : 'error'} />} />
          <StatusCard label="CORS to /exams" value={cors} icon={<AssessmentIcon color={cors === 'ok' ? 'success' : 'warning'} />} />
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="subtitle1" fontWeight={600}>Environment</Typography>
        <Table size="small" sx={{ mt: 1 }}>
          <TableBody>
            <TableRow><TableCell>API Base</TableCell><TableCell sx={{ fontFamily: 'monospace' }}>{API_BASE}</TableCell></TableRow>
            <TableRow><TableCell>JWT present</TableCell><TableCell>{jwt ? 'yes' : 'no'}</TableCell></TableRow>
          </TableBody>
        </Table>
      </Paper>

      {snack.node}
    </Stack>
  );
}

function StatusCard({ label, value, icon }: { label: string; value: string; icon?: React.ReactNode; }) {
  const color = value === 'ok' ? 'success.main' : value === 'down' ? 'error.main' : 'warning.main';
  return (
    <Paper variant="outlined" sx={{ p: 2, minWidth: 220 }}>
      <Stack direction="row" spacing={1.5} alignItems="center">
        {icon}
        <Box>
          <Typography fontWeight={600}>{label}</Typography>
          <Typography variant="body2" sx={{ color }}>{value}</Typography>
        </Box>
      </Stack>
    </Paper>
  );
}

/* -------------------- Identity & Roles -------------------- */
function IdentityRolesPanel({ jwt }: { jwt: string; }) {
  const [role, setRole] = useState<string>("");
  const [users, setUsers] = useState<User[]>([]);
  const [busy, setBusy] = useState(false);
  const [providers, setProviders] = useState<any[]>([]);
  const [apiKeys, setApiKeys] = useState<any[]>([]);
  const [newProviderJson, setNewProviderJson] = useState<string>("{\n  \"type\": \"oidc\",\n  \"client_id\": \"...\",\n  \"issuer\": \"https://...\"\n}");
  const [newApiKeyNote, setNewApiKeyNote] = useState<string>("");
  const snack = useSnack();

  async function fetchUsers() {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const qs = role ? `?${new URLSearchParams({ role }).toString()}` : "";
      const data = await api<User[]>(`/users${qs}`, { headers: { Authorization: `Bearer ${jwt}` } });
      setUsers(data);
    } catch (e: any) { snack.setErr(e.message); } finally { setBusy(false); }
  }
  useEffect(() => { fetchUsers(); // eslint-disable-next-line
  }, []);

  async function updateUserRole(uid: string, r: string) {
    try {
      const res = await fetch(`${API_BASE}/admin/users/${encodeURIComponent(uid)}`, {
        method: 'PATCH', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ role: r })
      });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg('Role updated');
      fetchUsers();
    } catch (e: any) { snack.setErr(e.message); }
  }

  async function bulkUpload(file: File) {
    try {
      const form = new FormData();
      form.append("file", file);
      const res = await fetch(`${API_BASE}/users/bulk`, { method: "POST", headers: { Authorization: `Bearer ${jwt}` }, body: form });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      snack.setMsg(`Upserted: +${data.inserted} / updated: ${data.updated}`);
      fetchUsers();
    } catch (e: any) { snack.setErr(e.message); }
  }

  async function loadProviders() {
    try { setProviders(await api<any[]>(`/admin/identity/providers`, { headers: { Authorization: `Bearer ${jwt}` } })); } catch { /* optional */ }
  }
  async function addProvider() {
    try {
      const payload = JSON.parse(newProviderJson);
      const res = await fetch(`${API_BASE}/admin/identity/providers`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify(payload) });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg('Provider added');
      setNewProviderJson("{}");
      loadProviders();
    } catch (e: any) { snack.setErr(e.message?.startsWith('Unexpected token') ? 'Invalid JSON' : e.message); }
  }

  async function loadApiKeys() {
    try { setApiKeys(await api<any[]>(`/admin/api-keys`, { headers: { Authorization: `Bearer ${jwt}` } })); } catch { /* optional */ }
  }
  async function createApiKey() {
    try {
      const res = await fetch(`${API_BASE}/admin/api-keys`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ note: newApiKeyNote }) });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      snack.setMsg(`Key created: ${data.prefix || '(see server)'}`);
      setNewApiKeyNote("");
      loadApiKeys();
    } catch (e: any) { snack.setErr(e.message); }
  }
  async function revokeApiKey(id: string) {
    try {
      const res = await fetch(`${API_BASE}/admin/api-keys/${encodeURIComponent(id)}`, { method: 'DELETE', headers: { Authorization: `Bearer ${jwt}` } });
      if (!res.ok && res.status !== 204) throw new Error(await res.text());
      snack.setMsg('Key revoked');
      loadApiKeys();
    } catch (e: any) { snack.setErr(e.message); }
  }

  useEffect(() => { loadProviders(); loadApiKeys(); }, []);

  return (
    <Stack spacing={3}>
      {/* Users & roles */}
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Grid container spacing={1.5} alignItems="flex-end">
          <Grid size={{ xs: 12, sm: "auto" }}>
            <FormControl sx={{ minWidth: 180 }}>
              <InputLabel id="role-filter">Filter by role</InputLabel>
              <Select labelId="role-filter" label="Filter by role" value={role} onChange={(e) => setRole(e.target.value)}>
                <MenuItem value="">(all)</MenuItem>
                <MenuItem value="student">student</MenuItem>
                <MenuItem value="teacher">teacher</MenuItem>
                <MenuItem value="admin">admin</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{ xs: 12, sm: "auto" }}>
            <Button variant="outlined" onClick={fetchUsers} disabled={busy}>{busy ? 'Loading…' : 'Refresh'}</Button>
          </Grid>
          <Grid sx={{ display: { xs: 'none', sm: 'block' } }}>
            <Divider flexItem orientation="vertical" />
          </Grid>
          <Grid size={{ xs: 12, sm: "auto" }}>
            <Button component="label" startIcon={<UploadFileIcon />} variant="contained" disableElevation>
              Bulk Upload CSV/JSON
              <input type="file" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) bulkUpload(f); e.currentTarget.value = ""; }} />
            </Button>
          </Grid>
        </Grid>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack spacing={1.25} sx={{ maxHeight: 420, overflowY: 'auto' }}>
          {users.length === 0 && !busy && (<Typography variant="body2" color="text.secondary">No users found.</Typography>)}
          {users.map((u) => (
            <Paper key={u.id} variant="outlined" sx={{ p: 1.25 }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
                <Box sx={{ flexGrow: 1 }}>
                  <Typography fontWeight={600}>{u.username}</Typography>
                  <Typography variant="caption" color="text.secondary">ID: <Box component="span" sx={{ fontFamily: 'monospace' }}>{u.id}</Box> • Role: {u.role}</Typography>
                </Box>
                <FormControl sx={{ minWidth: 140 }}>
                  <InputLabel id={`role-${u.id}`}>Set role</InputLabel>
                  <Select labelId={`role-${u.id}`} label="Set role" value={u.role} onChange={(e) => updateUserRole(u.id, String(e.target.value))}>
                    <MenuItem value="student">student</MenuItem>
                    <MenuItem value="teacher">teacher</MenuItem>
                    <MenuItem value="admin">admin</MenuItem>
                  </Select>
                </FormControl>
              </Stack>
            </Paper>
          ))}
        </Stack>
      </Paper>

      {/* SSO Providers */}
      {ENABLE_SSO_PROVIDERS && (
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>SSO Providers</Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }} sx={{ mt: 1.5 }}>
          <TextField label="Provider JSON" value={newProviderJson} onChange={(e) => setNewProviderJson(e.target.value)} multiline minRows={3} fullWidth />
          <Button variant="contained" disableElevation onClick={addProvider}>Add</Button>
          <Button variant="outlined" onClick={loadProviders}>Refresh</Button>
        </Stack>
        <Stack spacing={1} sx={{ mt: 1.5 }}>
          {providers.map((p, i) => (
            <Paper key={i} variant="outlined" sx={{ p: 1.25 }}>
              <Typography variant="caption" color="text.secondary">{JSON.stringify(p)}</Typography>
            </Paper>
          ))}
          {providers.length === 0 && (<Typography variant="body2" color="text.secondary">No providers configured.</Typography>)}
        </Stack>
      </Paper>
      )}
      {/* API Keys */}
      { ENABLE_API_KEYS && (
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>API Keys</Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }} sx={{ mt: 1.5 }}>
          <TextField label="Note (purpose)" value={newApiKeyNote} onChange={(e) => setNewApiKeyNote(e.target.value)} sx={{ minWidth: 280 }} />
          <Button variant="contained" disableElevation onClick={createApiKey} disabled={!newApiKeyNote.trim()}>Create</Button>
          <Button variant="outlined" onClick={loadApiKeys}>Refresh</Button>
        </Stack>
        <Table size="small" sx={{ mt: 1 }}>
          <TableHead>
            <TableRow>
              <TableCell>ID</TableCell>
              <TableCell>Prefix</TableCell>
              <TableCell>Note</TableCell>
              <TableCell>Created</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {apiKeys.map((k: any) => (
              <TableRow key={k.id} hover>
                <TableCell sx={{ fontFamily: 'monospace' }}>{k.id}</TableCell>
                <TableCell sx={{ fontFamily: 'monospace' }}>{k.prefix}</TableCell>
                <TableCell>{k.note}</TableCell>
                <TableCell>{k.created_at ? ms2date(k.created_at) : ''}</TableCell>
                <TableCell align="right"><Button color="error" onClick={() => revokeApiKey(k.id)}>Revoke</Button></TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>
      )}
      {snack.node}
    </Stack>
  );
}

/* -------------------- Content Governance -------------------- */
/* -------------------- Content Governance -------------------- */
function ContentGovernancePanel({ jwt }: { jwt: string; }) {
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<string>("pending");
  const [list, setList] = useState<ExamSummary[]>([]);
  const [viewExam, setViewExam] = useState<Exam | null>(null);

  // NEW: delete exam confirm + progress
  const [confirmDelExam, setConfirmDelExam] = useState<{ id: string; title: string } | null>(null);
  const [deletingExam, setDeletingExam] = useState(false);

  // NEW: courses display + delete (admin view)
  const [courses, setCourses] = useState<Course[]>([]);
  const [selectedCourseId, setSelectedCourseId] = useState<string>("");
  const [confirmDelCourse, setConfirmDelCourse] = useState(false);
  const [deletingCourse, setDeletingCourse] = useState(false);
  const [busyCourses, setBusyCourses] = useState(false);

  const snack = useSnack();

  const fetchExams = useCallback(async () => {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const params = new URLSearchParams();
      if (q.trim()) params.set('q', q.trim());
      if (status) params.set('status', status);
      const data = await api<ExamSummary[]>(`/exams?${params.toString()}`, { headers: { Authorization: `Bearer ${jwt}` } });
      setList(data);
    } catch (err: any) { snack.setErr(err.message); } finally { setBusy(false); }
  }, [jwt, q, status]);

  useEffect(() => { fetchExams(); }, [fetchExams]);

  async function openExam(id: string) {
    try { const data = await api<Exam>(`/exams/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${jwt}` } }); setViewExam(data); }
    catch (e: any) { snack.setErr(e.message); }
  }
  async function exportQTI(id: string) {
    try {
      const url = `${API_BASE}/exams/${encodeURIComponent(id)}/export?format=qti`;
      const res = await fetch(url, { headers: { Authorization: `Bearer ${jwt}` } });
      if (!res.ok) throw new Error(await res.text());
      const blob = await res.blob();
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `${id}.zip`;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (e: any) { snack.setErr(e.message); }
  }

  // NEW: delete exam
  async function doDeleteExam() {
    if (!confirmDelExam) return;
    setDeletingExam(true); snack.setErr(null); snack.setMsg(null);
    try {
      const res = await fetch(`${API_BASE}/exams/${encodeURIComponent(confirmDelExam.id)}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${jwt}` },
      });
      if (!res.ok && res.status !== 204) throw new Error(await res.text());
      snack.setMsg(`Deleted exam "${confirmDelExam.title}".`);
      setConfirmDelExam(null);
      fetchExams();
    } catch (e: any) {
      // e.g. server may block if attempts exist
      snack.setErr(e.message);
    } finally {
      setDeletingExam(false);
    }
  }

  // NEW: load & delete courses
  const loadCourses = useCallback(async () => {
    setBusyCourses(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Course[]>(`/courses`, { headers: { Authorization: `Bearer ${jwt}` } });
      setCourses(data);
      if (data.length > 0 && !selectedCourseId) setSelectedCourseId(data[0].id);
    } catch (e: any) {
      snack.setErr(e.message);
    } finally {
      setBusyCourses(false);
    }
  }, [jwt, selectedCourseId]);

  useEffect(() => { loadCourses(); }, [loadCourses]);

  async function deleteCourse() {
    if (!selectedCourseId) return;
    setDeletingCourse(true); snack.setErr(null); snack.setMsg(null);
    try {
      const res = await fetch(`${API_BASE}/courses/${encodeURIComponent(selectedCourseId)}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${jwt}` },
      });
      if (!res.ok && res.status !== 204) throw new Error(await res.text());
      snack.setMsg("Course deleted");
      setConfirmDelCourse(false);
      setSelectedCourseId("");
      loadCourses();
    } catch (e: any) {
      // e.g., deletion blocked if offerings/attempts exist
      snack.setErr(e.message);
    } finally {
      setDeletingCourse(false);
    }
  }

  return (
    <Stack spacing={3}>
      {/* --- Exams: search & filter --- */}
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Grid container spacing={1.5} alignItems="flex-end">
          <Grid size={{ xs: 12, sm: 6, md: 'auto' }}>
            <TextField label="Search exams" placeholder="title contains…" value={q} onChange={(e) => setQ(e.target.value)} fullWidth />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 'auto' }}>
            <FormControl sx={{ minWidth: 180 }}>
              <InputLabel id="status-filter">Status</InputLabel>
              <Select labelId="status-filter" label="Status" value={status} onChange={(e) => setStatus(String(e.target.value))}>
                <MenuItem value="pending">pending</MenuItem>
                <MenuItem value="approved">approved</MenuItem>
                <MenuItem value="archived">archived</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid>
            <Button variant="outlined" onClick={fetchExams} disabled={busy}>{busy ? 'Searching…' : 'Search'}</Button>
          </Grid>
          <Grid>
            <Button variant="text" onClick={() => { setQ(""); setStatus("pending"); fetchExams(); }} disabled={busy}>Reset</Button>
          </Grid>
        </Grid>
      </Paper>

      {/* --- Exams: list with Preview / Export / Delete --- */}
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack spacing={1.25} sx={{ maxHeight: 420, overflowY: 'auto' }}>
          {list.length === 0 && !busy && (<Typography variant="body2" color="text.secondary">No exams found.</Typography>)}
          {list.map((e) => (
            <Paper key={e.id} variant="outlined" sx={{ p: 1.5 }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'center' }}>
                <Box sx={{ flexGrow: 1 }}>
                  <Typography fontWeight={600}>{e.title}</Typography>
                  <Typography variant="caption" color="text.secondary">
                    <Box component="span" sx={{ fontFamily: 'monospace' }}>{e.id}</Box>
                    {typeof e.time_limit_sec === 'number' && <> • ⏱ {Math.round((e.time_limit_sec || 0)/60)} min</>}
                    {e.profile && <> • {e.profile}</>}
                    {e.created_at && <> • {ms2date(e.created_at)}</>}
                    {e.status && <> • Status: {e.status}</>}
                  </Typography>
                </Box>
                <Button variant="text" onClick={() => openExam(e.id)}>Preview</Button>
                <Button variant="outlined" startIcon={<FileDownloadIcon />} onClick={() => exportQTI(e.id)}>Export QTI</Button>
                {/* NEW: Delete exam */}
                <Button variant="outlined" color="error" onClick={() => setConfirmDelExam({ id: e.id, title: e.title })}>Delete</Button>
              </Stack>
            </Paper>
          ))}
        </Stack>
      </Paper>

      {/* --- Courses: display + delete (admin) --- */}
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>Courses</Typography>
        <Grid container spacing={1.5} alignItems="flex-end" sx={{ mt: 1 }}>
          <Grid size={{ xs: 12, md: 6 }}>
            <FormControl fullWidth>
              <InputLabel id="course-select-admin">Select course</InputLabel>
              <Select
                labelId="course-select-admin"
                label="Select course"
                value={selectedCourseId}
                onChange={(e) => setSelectedCourseId(String(e.target.value))}
              >
                {courses.map((c) => (
                  <MenuItem key={c.id} value={c.id}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Box component="span" sx={{ fontFamily: "monospace", fontSize: 12 }}>{c.id}</Box>
                      <Box component="span">• {c.name}</Box>
                    </Box>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid>
            <Button variant="outlined" onClick={loadCourses} disabled={busyCourses}>{busyCourses ? "…" : "Refresh"}</Button>
          </Grid>
          <Grid>
            <Button
              variant="outlined"
              color="error"
              onClick={() => setConfirmDelCourse(true)}
              disabled={!selectedCourseId}
            >
              Delete Course
            </Button>
          </Grid>
        </Grid>

        {/* small table listing for visibility */}
        <Table size="small" sx={{ mt: 2 }}>
          <TableHead>
            <TableRow>
              <TableCell>Course ID</TableCell>
              <TableCell>Name</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {courses.map((c) => (
              <TableRow key={c.id} hover selected={c.id === selectedCourseId} onClick={() => setSelectedCourseId(c.id)} sx={{ cursor: 'pointer' }}>
                <TableCell sx={{ fontFamily: 'monospace' }}>{c.id}</TableCell>
                <TableCell>{c.name}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>

      {/* Exam preview dialog */}
      <Dialog open={!!viewExam} onClose={() => setViewExam(null)} maxWidth="md" fullWidth>
        <DialogTitle>Preview: {viewExam?.title}</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={2}>
            {viewExam?.questions.map((q, i) => (
              <Paper key={q.id} variant="outlined" sx={{ p: 2 }}>
                <Typography variant="caption" color="text.secondary">Q{i+1} • {q.type}</Typography>
                <Box sx={{ mt: 1, '& p': { my: 0.5 } }} dangerouslySetInnerHTML={htmlify(q.prompt_html || q.prompt)} />
              </Paper>
            ))}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setViewExam(null)}>Close</Button>
        </DialogActions>
      </Dialog>

      {/* NEW: Confirm delete exam */}
      <Dialog open={!!confirmDelExam} onClose={() => !deletingExam && setConfirmDelExam(null)} maxWidth="xs" fullWidth>
        <DialogTitle>Delete exam?</DialogTitle>
        <DialogContent dividers>
          <DialogContentText>
            {`This will permanently remove exam "${confirmDelExam?.title}". `}
            Deletion may be blocked if attempts exist—use Archive instead.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDelExam(null)} disabled={deletingExam}>Cancel</Button>
          <Button variant="contained" color="error" disableElevation onClick={doDeleteExam} disabled={deletingExam}>
            {deletingExam ? "Deleting…" : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* NEW: Confirm delete course */}
      <Dialog open={confirmDelCourse} onClose={() => !deletingCourse && setConfirmDelCourse(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Delete course?</DialogTitle>
        <DialogContent dividers>
          <DialogContentText>
            {selectedCourseId
              ? <>You’re about to delete course <code>{selectedCourseId}</code>.</>
              : "No course selected."}
            {" "}This removes enrollments and offerings. If attempts exist, deletion will be blocked.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDelCourse(false)} disabled={deletingCourse}>Cancel</Button>
          <Button variant="contained" color="error" disableElevation onClick={deleteCourse} disabled={deletingCourse || !selectedCourseId}>
            {deletingCourse ? "Deleting…" : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>

      {snack.node}
    </Stack>
  );
}


/* -------------------- Attempts Oversight -------------------- */
function AttemptsOversightPanel({ jwt }: { jwt: string; }) {
  const [attemptId, setAttemptId] = useState("");
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const [busy, setBusy] = useState(false);
  const [reason, setReason] = useState("");
  const snack = useSnack();

  async function loadAttempt(id: string) {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try { const data = await api<Attempt>(`/attempts/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${jwt}` } }); setAttempt(data); snack.setMsg(`Loaded attempt ${data.id}`); }
    catch (e: any) { snack.setErr(e.message); } finally { setBusy(false); }
  }

  async function adminAct(kind: 'force-submit'|'invalidate'|'unlock') {
    if (!attempt?.id || !reason.trim()) { snack.setErr('Reason is required'); return; }
    try {
      const res = await fetch(`${API_BASE}/admin/attempts/${encodeURIComponent(attempt.id)}/${kind}`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ reason }) });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg(`Action ${kind} queued`);
      setReason("");
      loadAttempt(attempt.id);
    } catch (e: any) { snack.setErr(e.message); }
  }

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Grid container spacing={1.5} alignItems="flex-end">
          <Grid size={{ xs: 12, sm: 6, md: 'auto' }}>
            <TextField label="Attempt ID" value={attemptId} onChange={(e) => setAttemptId(e.target.value)} fullWidth />
          </Grid>
          <Grid>
            <Button variant="contained" disableElevation onClick={() => loadAttempt(attemptId)} disabled={!attemptId.trim() || busy}>{busy ? "Loading…" : "Open"}</Button>
          </Grid>
          <Grid sx={{ flexGrow: 1 }} />
          <Grid size={{ xs: 12, md: 6 }}>
            <TextField label="Reason (required for admin overrides)" value={reason} onChange={(e) => setReason(e.target.value)} fullWidth />
          </Grid>
        </Grid>
      </Paper>

      {attempt && (
        <Paper elevation={1} sx={{ p: 2.5 }}>
          <Stack spacing={1}>
            <Typography variant="h6" fontWeight={600}>Attempt {attempt.id}</Typography>
            <Typography variant="body2" color="text.secondary">Exam: <Box component="span" sx={{ fontFamily: 'monospace' }}>{attempt.exam_id}</Box> • User: {attempt.user_id} • Status: {attempt.status} {typeof attempt.score === 'number' && (<Chip size="small" color="success" label={`Score: ${attempt.score}`} sx={{ ml: 1 }} />)}</Typography>
            <Divider sx={{ my: 1 }} />
            <Typography variant="subtitle2">Admin overrides</Typography>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
              <Button variant="outlined" onClick={() => adminAct('force-submit')} disabled={!reason.trim()}>Force submit</Button>
              <Button variant="outlined" color="warning" onClick={() => adminAct('unlock')} disabled={!reason.trim()}>Unlock</Button>
              <Button variant="outlined" color="error" onClick={() => adminAct('invalidate')} disabled={!reason.trim()}>Invalidate</Button>
            </Stack>
            <Divider sx={{ my: 1 }} />
            <Typography variant="subtitle2">Responses (raw)</Typography>
            <Paper variant="outlined" sx={{ p: 2, bgcolor: 'grey.50', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace', whiteSpace: 'pre-wrap' }}>
              {JSON.stringify(attempt.responses ?? {}, null, 2)}
            </Paper>
          </Stack>
        </Paper>
      )}

      {snack.node}
    </Stack>
  );
}

/* -------------------- Tenants & Feature Flags -------------------- */
function TenantsFlagsPanel({ jwt }: { jwt: string }) {
  const snack = useSnack();
  const [busy, setBusy] = useState(false);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [newTenantName, setNewTenantName] = useState("");
  const [newTenantDomain, setNewTenantDomain] = useState("");

  async function load() {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try { setTenants(await api<Tenant[]>(`/admin/tenants`, { headers: { Authorization: `Bearer ${jwt}` } })); }
    catch (e: any) { snack.setErr(e.message); } finally { setBusy(false); }
  }
  useEffect(() => { load(); }, []);

  async function createTenant() {
    try {
      const res = await fetch(`${API_BASE}/admin/tenants`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ name: newTenantName.trim(), domain: newTenantDomain.trim() || undefined }) });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg('Tenant created');
      setNewTenantName(""); setNewTenantDomain("");
      load();
    } catch (e: any) { snack.setErr(e.message); }
  }

  async function saveFlags(t: Tenant) {
    try {
      const res = await fetch(`${API_BASE}/admin/tenants/${encodeURIComponent(t.id)}/flags`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify(t.flags || {}) });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg('Flags updated');
    } catch (e: any) { snack.setErr(e.message); }
  }

  function toggleFlag(idx: number, key: string) {
    setTenants(prev => prev.map((t, i) => i !== idx ? t : ({ ...t, flags: { ...(t.flags || {}), [key]: !(t.flags?.[key]) } })));
  }

  const knownFlags = [
    { key: 'lti_enabled', label: 'LTI enabled' },
    { key: 'qti_import', label: 'QTI import' },
    { key: 'public_offerings', label: 'Public offerings' },
    { key: 'link_visibility', label: 'Link visibility' },
  ];

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Grid container spacing={1.5} alignItems="flex-end">
          <Grid size={{ xs: 12, md: 4 }}><TextField label="New tenant name" value={newTenantName} onChange={(e) => setNewTenantName(e.target.value)} fullWidth /></Grid>
          <Grid size={{ xs: 12, md: 4 }}><TextField label="Custom domain (optional)" value={newTenantDomain} onChange={(e) => setNewTenantDomain(e.target.value)} fullWidth /></Grid>
          <Grid><Button variant="contained" disableElevation onClick={createTenant} disabled={!newTenantName.trim()}>Create</Button></Grid>
          <Grid><Button variant="outlined" onClick={load} disabled={busy}>{busy ? '…' : 'Refresh'}</Button></Grid>
        </Grid>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack spacing={1.25} sx={{ maxHeight: 480, overflowY: 'auto' }}>
          {tenants.length === 0 && !busy && (<Typography variant="body2" color="text.secondary">No tenants found.</Typography>)}
          {tenants.map((t, idx) => (
            <Paper key={t.id} variant="outlined" sx={{ p: 1.25 }}>
              <Stack spacing={1}>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
                  <Box sx={{ flexGrow: 1 }}>
                    <Typography fontWeight={600}>{t.name}</Typography>
                    <Typography variant="caption" color="text.secondary">ID: <Box component="span" sx={{ fontFamily: 'monospace' }}>{t.id}</Box> {t.domain && <>• Domain: {t.domain}</>}</Typography>
                  </Box>
                  <Button variant="outlined" onClick={() => saveFlags(t)}>Save flags</Button>
                </Stack>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                  {knownFlags.map(f => (
                    <FormControlLabel key={f.key} control={<Switch checked={!!t.flags?.[f.key]} onChange={() => toggleFlag(idx, f.key)} />} label={f.label} />
                  ))}
                </Stack>
              </Stack>
            </Paper>
          ))}
        </Stack>
      </Paper>

      {snack.node}
    </Stack>
  );
}

/* -------------------- Compliance & Audit -------------------- */
function ComplianceAuditPanel({ jwt }: { jwt: string; }) {
  const snack = useSnack();
  const [userId, setUserId] = useState("");
  const [auditQuery, setAuditQuery] = useState("");
  const [auditRows, setAuditRows] = useState<any[]>([]);

  async function piiAct(kind: 'export'|'delete') {
    if (!userId.trim()) { snack.setErr('User ID required'); return; }
    try {
      const res = await fetch(`${API_BASE}/admin/pii/${kind}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${jwt}`,
        },
        body: JSON.stringify({ user_id: userId.trim() }),
      });
      if (!res.ok) throw new Error(await res.text());
  
      if (kind === 'export') {
        const blob = await res.blob();
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = `pii_${userId}.json`;
        a.click();
        URL.revokeObjectURL(a.href);
        snack.setMsg("PII export downloaded.");
        return;
      }
  
      if (kind === 'delete') {
        const data = await res.json();
        snack.setMsg(`Deleted user: ${data.status}`);
      }
    } catch (e: any) {
      snack.setErr(e.message);
    }
  }

  async function searchAudit() {
    try {
      const params = new URLSearchParams();
      if (auditQuery.trim()) params.set('q', auditQuery.trim());
      const rows = await api<any[]>(`/admin/audit?${params.toString()}`, { headers: { Authorization: `Bearer ${jwt}` } });
      setAuditRows(rows || []);   // <-- ensure array
    } catch (e: any) {
      snack.setErr(e.message);
      setAuditRows([]);           // <-- keep safe state
    }
  }

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>PII Tools</Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }} sx={{ mt: 1.5 }}>
          <TextField label="User ID" value={userId} onChange={(e) => setUserId(e.target.value)} sx={{ minWidth: 260 }} />
          <Button variant="outlined" onClick={() => piiAct('export')}>Export</Button>
          <Button variant="outlined" color="error" onClick={() => piiAct('delete')}>Delete</Button>
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>Audit Log</Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }} sx={{ mt: 1.5 }}>
          <TextField label="Query (actor:*, action:*, target:*)" value={auditQuery} onChange={(e) => setAuditQuery(e.target.value)} fullWidth />
          <Button variant="outlined" onClick={searchAudit}>Search</Button>
        </Stack>
        <Table size="small" sx={{ mt: 1 }}>
          <TableHead>
            <TableRow>
              <TableCell>At</TableCell>
              <TableCell>Actor</TableCell>
              <TableCell>Action</TableCell>
              <TableCell>Target</TableCell>
              <TableCell>Reason</TableCell>
              <TableCell>IP</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
          {(auditRows || []).length === 0 && (
            <TableRow>
              <TableCell colSpan={6}>
                <Typography variant="body2" color="text.secondary">
                  No audit entries found for this query.
                </Typography>
              </TableCell>
            </TableRow>
          )}
          {(auditRows || []).map((r, i) => (
            <TableRow key={i}>
              <TableCell>{r.at ? new Date(r.at).toLocaleString() : ''}</TableCell>
              <TableCell>{r.actor}</TableCell>
              <TableCell>{r.action}</TableCell>
              <TableCell>{r.target}</TableCell>
              <TableCell>{r.reason}</TableCell>
              <TableCell>{r.ip}</TableCell>
            </TableRow>
          ))}
        </TableBody>

        </Table>
      </Paper>

      {snack.node}
    </Stack>
  );
}

/* -------------------- Integrations (kept scaffold) -------------------- */
function IntegrationsPanel({ jwt }: { jwt: string; }) {
  const [jwks, setJwks] = useState<null | { keys: any[] }>(null);
  const [ltiEnabled, setLtiEnabled] = useState<boolean | null>(null);

  useEffect(() => {
    (async () => {
      try { const res = await fetch(`${API_BASE}/.well-known/jwks.json`); setJwks(res.ok ? await res.json() : null); } catch { setJwks(null); }
      try { const res = await fetch(`${API_BASE}/lti/login`); setLtiEnabled(res.redirected || res.status === 302 || res.status === 200); } catch { setLtiEnabled(false); }
    })();
  }, [jwt]);

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>JWKS</Typography>
        <Typography variant="body2" color="text.secondary">{jwks ? `Available (${jwks.keys?.length || 0} keys)` : 'Not available (feature-flagged or offline mode).'} </Typography>
      </Paper>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>LTI</Typography>
        <Stack direction="row" spacing={2} alignItems="center" sx={{ mt: 1 }}>
          <Typography>Login/Launch endpoints</Typography>
          <Switch checked={!!ltiEnabled} disabled />
          <Typography variant="caption" color="text.secondary">(probe only; configure in server env)</Typography>
        </Stack>
      </Paper>
      <Paper elevation={0} sx={{ p: 2.5 }}>
        <Typography variant="caption" color="text.secondary">Note: Admin actions like key rotation, platform registry, and policy templates may require backend endpoints. This panel is a safe UI scaffold.</Typography>
      </Paper>
    </Stack>
  );
}

/* -------------------- Settings (CORS, IP allowlist, Branding) -------------------- */
function SettingsPanel({ jwt }: { jwt: string }) {
  const snack = useSnack();
  const [origins, setOrigins] = useState<string>("");
  const [ips, setIps] = useState<string>("");
  const [brandName, setBrandName] = useState<string>("");
  const [primaryColor, setPrimaryColor] = useState<string>("#3f51b5");

  async function load() {
    try {
      const o = await api<{ origins: string[] }>(`/admin/cors`, { headers: { Authorization: `Bearer ${jwt}` } }).catch(() => ({ origins: [] } as any));
      const i = await api<{ ips: string[] }>(`/admin/ip-allowlist`, { headers: { Authorization: `Bearer ${jwt}` } }).catch(() => ({ ips: [] } as any));
      setOrigins((o?.origins || []).join("\n"));
      setIps((i?.ips || []).join("\n"));
    } catch { /* non-fatal */ }
  }
  useEffect(() => { load(); }, []);

  async function saveCors() {
    try {
      const res = await fetch(`${API_BASE}/admin/cors`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ origins: origins.split(/\n+/).map(s => s.trim()).filter(Boolean) }) });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg('CORS updated');
    } catch (e: any) { snack.setErr(e.message); }
  }
  async function saveIps() {
    try {
      const res = await fetch(`${API_BASE}/admin/ip-allowlist`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ ips: ips.split(/\n+/).map(s => s.trim()).filter(Boolean) }) });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg('IP allowlist updated');
    } catch (e: any) { snack.setErr(e.message); }
  }
  async function saveBranding() {
    try {
      const res = await fetch(`${API_BASE}/admin/branding`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ name: brandName || undefined, primary_color: primaryColor }) });
      if (!res.ok) throw new Error(await res.text());
      snack.setMsg('Branding saved');
    } catch (e: any) { snack.setErr(e.message); }
  }

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>CORS Origins</Typography>
        <Stack spacing={1.5} sx={{ mt: 1.5 }}>
          <TextField label="Allowed origins (one per line)" value={origins} onChange={(e) => setOrigins(e.target.value)} multiline minRows={4} fullWidth />
          <Button variant="contained" disableElevation onClick={saveCors}>Save</Button>
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>IP Allowlist</Typography>
        <Stack spacing={1.5} sx={{ mt: 1.5 }}>
          <TextField label="Allowed CIDRs/IPs (one per line)" value={ips} onChange={(e) => setIps(e.target.value)} multiline minRows={4} fullWidth />
          <Button variant="contained" disableElevation onClick={saveIps}>Save</Button>
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Typography variant="h6" fontWeight={600}>Branding</Typography>
        <Grid container spacing={1.5} alignItems="flex-end" sx={{ mt: 1.5 }}>
          <Grid size={{ xs: 12, md: 6 }}><TextField label="Display name" value={brandName} onChange={(e) => setBrandName(e.target.value)} fullWidth /></Grid>
          <Grid size={{ xs: 12, md: 3 }}><TextField label="Primary color" type="color" value={primaryColor} onChange={(e) => setPrimaryColor(e.target.value)} fullWidth /></Grid>
          <Grid><Button variant="contained" disableElevation onClick={saveBranding}>Save</Button></Grid>
        </Grid>
      </Paper>
    </Stack>
  );
}

/* -------------------- Root -------------------- */
export default function AdminApp() {
  type Screen = "login" | "home";
  const [screen, setScreen] = useState<Screen>("login");
  const [jwt, setJwt] = useState("");
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState(0);
  const snack = useSnack();

  const [features, setFeatures] = useState<Features | null>(null);

  async function login(username: string, password: string) {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<{ access_token: string }>("/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role: "admin" }),
      });
      setJwt(data.access_token);
      try { localStorage.setItem("admin_jwt", data.access_token); } catch {}
      setScreen("home");
      snack.setMsg("Logged in.");
    } catch (err: any) {
      snack.setErr(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function validateToken(token: string) {
    try {
      // Replace `/users/me` with `/me` if your backend uses that path
      const res = await fetch(`${API_BASE}/users/me`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) throw new Error("invalid");
      return true;
    } catch {
      return false;
    }
  }

  useEffect(() => {
    (async () => {
      try {
        const f = await api<Features>("/features");
        setFeatures(f);
      } catch {
        setFeatures({ mode: "offline", enable_google_auth: false }); // safe fallback
      }
    })();
  }, []);

  // Restore token on mount + validate it
  useEffect(() => {
    (async () => {
      let token = "";
      const url = new URL(window.location.href);

      token =
        url.searchParams.get("access_token") ||
        url.searchParams.get("jwt") ||
        "";

      if (!token && window.location.hash) {
        const hashParams = new URLSearchParams(window.location.hash.slice(1));
        token = hashParams.get("access_token") || hashParams.get("jwt") || "";
      }
      if (!token) {
        try {
          token = localStorage.getItem("admin_jwt") || "";
        } catch {}
      }

      if (token) {
        const valid = await validateToken(token);
        if (valid) {
          setJwt(token);
          try { localStorage.setItem("admin_jwt", token); } catch {}
          setScreen("home");
        } else {
          // force logout if invalid
          try { localStorage.removeItem("admin_jwt"); } catch {}
          setJwt("");
          setScreen("login");
          snack.setErr("Session expired. Please log in again.");
        }

        // cleanup URL so token isn't exposed
        url.searchParams.delete("access_token");
        url.searchParams.delete("jwt");
        window.history.replaceState(
          {},
          document.title,
          url.pathname + (url.search ? `?${url.searchParams.toString()}` : "")
        );
        if (window.location.hash) {
          window.history.replaceState(
            {},
            document.title,
            url.pathname + (url.search ? `?${url.searchParams.toString()}` : "")
          );
        }
      }
    })();
  }, []); // runs only once on mount

  function signOut() {
    setJwt("");
    try { localStorage.removeItem("admin_jwt"); } catch {}
    setScreen("login");
    snack.setMsg("Signed out.");
  }

  const theme = useMemo(
    () =>
      createTheme({
        palette: { mode: "light", primary: { main: "#3f51b5" } },
        shape: { borderRadius: 12 },
        components: {
          MuiPaper: { styleOverrides: { root: { borderRadius: 16 } } },
          MuiButton: { defaultProps: { disableRipple: true } },
        },
      }),
    []
  );

  if (screen === "login") {
    return (
      <ThemeProvider theme={theme}>
        <LoginScreen busy={busy} onLogin={login} features={features} />
      </ThemeProvider>
    );
  }

  return (
    <ThemeProvider theme={theme}>
      <Shell
        authed={true}
        onSignOut={signOut}
        title="Admin Console"
        right={
          <Tabs
            value={tab}
            onChange={(_, v) => setTab(v)}
            textColor="primary"
            indicatorColor="primary"
            sx={{ mr: 1 }}
          >
            <Tab icon={<SecurityIcon />} iconPosition="start" label="Overview" />
            <Tab disabled icon={<FlagIcon />} iconPosition="start" label="Tenants & Flags" />
            <Tab icon={<VerifiedUserIcon />} iconPosition="start" label="Identity & Roles" />
            <Tab icon={<LibraryBooksIcon />} iconPosition="start" label="Content" />
            <Tab icon={<FactCheckIcon />} iconPosition="start" label="Attempts" />
            <Tab icon={<GavelIcon />} iconPosition="start" label="Compliance" />
            <Tab disabled icon={<IntegrationInstructionsIcon />} iconPosition="start" label="Integrations" />
            <Tab disabled icon={<SettingsEthernetIcon />} iconPosition="start" label="Settings" />
          </Tabs>
        }
      >
        {tab === 0 && <OverviewPanel jwt={jwt} />}
        {tab === 1 && <TenantsFlagsPanel jwt={jwt} />}
        {tab === 2 && <IdentityRolesPanel jwt={jwt} />}
        {tab === 3 && <ContentGovernancePanel jwt={jwt} />}
        {tab === 4 && <AttemptsOversightPanel jwt={jwt} />}
        {tab === 5 && <ComplianceAuditPanel jwt={jwt} />}
        {tab === 6 && <IntegrationsPanel jwt={jwt} />}
        {tab === 7 && <SettingsPanel jwt={jwt} />}
        {snack.node}
      </Shell>
    </ThemeProvider>
  );
}

