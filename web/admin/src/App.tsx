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
  LinearProgress,
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
} from "@mui/material";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import LogoutIcon from "@mui/icons-material/Logout";
import UploadFileIcon from "@mui/icons-material/UploadFile";
import FileDownloadIcon from "@mui/icons-material/FileDownload";
import ManageAccountsIcon from "@mui/icons-material/ManageAccounts";
import IntegrationInstructionsIcon from "@mui/icons-material/IntegrationInstructions";
import AssessmentIcon from "@mui/icons-material/Assessment";
import SecurityIcon from "@mui/icons-material/Security";
import StorageIcon from "@mui/icons-material/Storage";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";

const API_BASE = (import.meta as any).env?.VITE_API_BASE || "http://localhost:8080";

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
};

export type Attempt = {
  id: string;
  exam_id: string;
  user_id: string;
  status: string;
  score?: number;
  responses?: Record<string, any>;
};

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
function LoginScreen({ busy, onLogin }: { busy: boolean; onLogin: (u: string, p: string) => void; }) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("admin");
  function submit(e: React.FormEvent) { e.preventDefault(); onLogin(username, password); }
  return (
    <Shell authed={false} title="Sign in">
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
        <Box sx={{ width: { xs: '100%', md: `${(7 / 12) * 100}%`, lg: '50%' } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="h5" fontWeight={600}>Admin Sign in</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>Use test creds (username=password). Role is enforced server-side by JWT.</Typography>
            <Box component="form" onSubmit={submit} sx={{ mt: 2 }}>
              <Stack spacing={2}>
                <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} fullWidth />
                <TextField label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} fullWidth />
                <Button type="submit" variant="contained" size="large" disableElevation disabled={busy}>{busy ? "…" : "Login"}</Button>
              </Stack>
            </Box>
          </Paper>
        </Box>
        <Box sx={{ width: { xs: '100%', md: `${(5 / 12) * 100}%`, lg: '50%' } }}>
          <Paper elevation={0} sx={{ p: 3, height: "100%" }}>
            <Typography variant="subtitle1" fontWeight={600}>Admin responsibilities</Typography>
            <Box component="ul" sx={{ mt: 1.5, pl: 3 }}>
              <li>User & role management</li>
              <li>Exam lifecycle (lock/archive/export)</li>
              <li>System health & integrations (LTI/JWKS)</li>
              <li>Audit & compliance</li>
            </Box>
          </Paper>
        </Box>
      </Stack>
    </Shell>
  );
}

/* -------------------- System Panel -------------------- */
function SystemPanel({ jwt }: { jwt: string; }) {
  const [health, setHealth] = useState<"unknown" | "ok" | "down">("unknown");
  const [ready, setReady] = useState<"unknown" | "ok" | "down">("unknown");
  const [cors, setCors] = useState<string>("(probe)");
  const snack = useSnack();

  async function ping(path: string): Promise<"ok" | "down"> {
    try { const res = await fetch(`${API_BASE}${path}`); return res.ok ? "ok" : "down"; } catch { return "down"; }
  }
  useEffect(() => {
    (async () => {
      setHealth(await ping("/healthz"));
      setReady(await ping("/readyz"));
      try {
        const res = await fetch(`${API_BASE}/exams`, { headers: { Authorization: `Bearer ${jwt}` } });
        setCors(res.ok ? "ok" : `blocked (${res.status})`);
      } catch { setCors("blocked"); }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

/* -------------------- Users Panel -------------------- */
function UsersPanel({ jwt }: { jwt: string; }) {
  const [role, setRole] = useState<string>("");
  const [users, setUsers] = useState<Array<{ id: string; username: string; role: string }>>([]);
  const [busy, setBusy] = useState(false);
  const snack = useSnack();

  async function fetchUsers() {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const url = new URL(`${API_BASE}/users`);
      if (role) url.searchParams.set("role", role);
      const res = await fetch(url.toString(), { headers: { Authorization: `Bearer ${jwt}` } });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      setUsers(data);
    } catch (e: any) { snack.setErr(e.message); } finally { setBusy(false); }
  }
  useEffect(() => { fetchUsers(); /* initial */ // eslint-disable-next-line
  }, []);

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

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }}>
          <FormControl sx={{ minWidth: 180 }}>
            <InputLabel id="role-filter">Filter by role</InputLabel>
            <Select labelId="role-filter" label="Filter by role" value={role} onChange={(e) => setRole(e.target.value)}>
              <MenuItem value="">(all)</MenuItem>
              <MenuItem value="student">student</MenuItem>
              <MenuItem value="teacher">teacher</MenuItem>
              <MenuItem value="admin">admin</MenuItem>
            </Select>
          </FormControl>
          <Button variant="outlined" onClick={fetchUsers} disabled={busy}>{busy ? 'Loading…' : 'Refresh'}</Button>
          <Divider flexItem orientation="vertical" sx={{ display: { xs: 'none', sm: 'block' } }} />
          <Button component="label" startIcon={<UploadFileIcon />} variant="contained" disableElevation>
            Bulk Upload CSV/JSON
            <input type="file" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) bulkUpload(f); e.currentTarget.value = ""; }} />
          </Button>
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 0 }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Username</TableCell>
              <TableCell>ID</TableCell>
              <TableCell>Role</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {users.map((u) => (
              <TableRow key={u.id} hover>
                <TableCell>{u.username}</TableCell>
                <TableCell sx={{ fontFamily: 'monospace' }}>{u.id}</TableCell>
                <TableCell>{u.role}</TableCell>
                <TableCell align="right">
                  <Tooltip title="Force reset (not implemented in API yet)"><span><Button size="small" disabled>Force reset</Button></span></Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>

      {snack.node}
    </Stack>
  );
}

/* -------------------- Exams Panel -------------------- */
function ExamsPanel({ jwt }: { jwt: string; }) {
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [list, setList] = useState<ExamSummary[]>([]);
  const [viewExam, setViewExam] = useState<Exam | null>(null);
  const snack = useSnack();

  const fetchExams = useCallback(async (query: string) => {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const url = new URL(`${API_BASE}/exams`);
      if (query.trim()) url.searchParams.set("q", query.trim());
      const res = await fetch(url.toString(), { headers: { Authorization: `Bearer ${jwt}` } });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}: ${await res.text()}`);
      const data: ExamSummary[] = await res.json();
      setList(data);
    } catch (err: any) { snack.setErr(err.message); } finally { setBusy(false); }
  }, [jwt]);

  useEffect(() => { fetchExams(""); }, [fetchExams]);

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

  async function importQTI(file: File) {
    try {
      const form = new FormData();
      form.append("file", file);
      const res = await fetch(`${API_BASE}/qti/import`, { method: "POST", headers: { Authorization: `Bearer ${jwt}` }, body: form });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      snack.setMsg(`Imported exam ${data.exam_id}`);
      fetchExams(q);
    } catch (e: any) { snack.setErr(e.message); }
  }

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }}>
          <Box sx={{ flexGrow: 1 }}>
            <TextField label="Search exams" placeholder="title contains…" value={q} onChange={(e) => setQ(e.target.value)} fullWidth />
          </Box>
          <Button variant="outlined" onClick={() => fetchExams(q)} disabled={busy}>{busy ? "Searching…" : "Search"}</Button>
          <Button variant="text" onClick={() => { setQ(""); fetchExams(""); }} disabled={busy}>Reset</Button>
          <Divider flexItem orientation="vertical" sx={{ display: { xs: 'none', sm: 'block' } }} />
          <Button component="label" startIcon={<UploadFileIcon />} variant="outlined">Import QTI<input type="file" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) importQTI(f); e.currentTarget.value = ""; }} /></Button>
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack spacing={1.25} sx={{ maxHeight: 480, overflowY: 'auto' }}>
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
                  </Typography>
                </Box>
                <Button variant="text" onClick={() => openExam(e.id)}>Preview</Button>
                <Button variant="outlined" startIcon={<FileDownloadIcon />} onClick={() => exportQTI(e.id)}>Export QTI</Button>
                <Tooltip title="Lock/Archive requires backend endpoints"><span><Button disabled>Lock</Button></span></Tooltip>
                <Tooltip title="Delete requires backend endpoint"><span><Button color="error" disabled>Delete</Button></span></Tooltip>
              </Stack>
            </Paper>
          ))}
        </Stack>
      </Paper>

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

      {snack.node}
    </Stack>
  );
}

/* -------------------- Attempts Panel -------------------- */
function AttemptsPanel({ jwt }: { jwt: string; }) {
  const [attemptId, setAttemptId] = useState("");
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const snack = useSnack();

  async function loadAttempt(id: string) {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try { const data = await api<Attempt>(`/attempts/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${jwt}` } }); setAttempt(data); snack.setMsg(`Loaded attempt ${data.id}`); }
    catch (e: any) { snack.setErr(e.message); } finally { setBusy(false); }
  }

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }}>
          <Box sx={{ flexGrow: 1 }}>
            <TextField label="Attempt ID" value={attemptId} onChange={(e) => setAttemptId(e.target.value)} fullWidth />
          </Box>
          <Button variant="contained" disableElevation onClick={() => loadAttempt(attemptId)} disabled={!attemptId.trim() || busy}>{busy ? "Loading…" : "Open"}</Button>
        </Stack>
      </Paper>

      {attempt && (
        <Paper elevation={1} sx={{ p: 2.5 }}>
          <Stack spacing={1}>
            <Typography variant="h6" fontWeight={600}>Attempt {attempt.id}</Typography>
            <Typography variant="body2" color="text.secondary">Exam: <Box component="span" sx={{ fontFamily: 'monospace' }}>{attempt.exam_id}</Box> • User: {attempt.user_id} • Status: {attempt.status} {typeof attempt.score === 'number' && (<Chip size="small" color="success" label={`Score: ${attempt.score}`} sx={{ ml: 1 }} />)}</Typography>
            <Divider sx={{ my: 1 }} />
            <Typography variant="subtitle2">Admin tools</Typography>
            <Stack direction="row" spacing={1.5}>
              <Tooltip title="Force submit requires backend endpoint"><span><Button variant="outlined" disabled>Force submit</Button></span></Tooltip>
              <Tooltip title="Reset attempt requires backend endpoint"><span><Button variant="outlined" disabled>Reset</Button></span></Tooltip>
              <Tooltip title="Invalidate requires backend endpoint"><span><Button variant="outlined" color="error" disabled>Invalidate</Button></span></Tooltip>
            </Stack>
            <Divider sx={{ my: 1 }} />
            <Typography variant="subtitle2">Responses (raw)</Typography>
            <Paper variant="outlined" sx={{ p: 2, bgcolor: 'grey.50', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace', whiteSpace: 'pre-wrap' }}>
              {JSON.stringify(attempt.responses ?? {}, null, 2)}
            </Paper>
          </Stack>
        </Paper>
      )}

      <Dialog open={confirm} onClose={() => setConfirm(false)}>
        <DialogTitle>Confirm action</DialogTitle>
        <DialogContent><DialogContentText>This is a placeholder until admin attempt endpoints exist.</DialogContentText></DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirm(false)}>Close</Button>
        </DialogActions>
      </Dialog>

      {snack.node}
    </Stack>
  );
}

/* -------------------- Integrations Panel -------------------- */
function IntegrationsPanel({ jwt }: { jwt: string; }) {
  const [jwks, setJwks] = useState<null | { keys: any[] }>(null);
  const [ltiEnabled, setLtiEnabled] = useState<boolean | null>(null);
  const snack = useSnack();

  useEffect(() => {
    (async () => {
      // Probe JWKS (online mode only)
      try {
        const res = await fetch(`${API_BASE}/.well-known/jwks.json`);
        if (res.ok) setJwks(await res.json()); else setJwks(null);
      } catch { setJwks(null); }
      // Probe LTI login (exists only if enabled)
      try {
        const res = await fetch(`${API_BASE}/lti/login`);
        setLtiEnabled(res.redirected || res.status === 302 || res.status === 200);
      } catch { setLtiEnabled(false); }
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
        <Typography variant="caption" color="text.secondary">Note: Admin actions like key rotation, platform registry, and policy templates will require new backend endpoints. This panel is a safe UI scaffold.</Typography>
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
  const [tab, setTab] = useState(0); // 0=System,1=Users,2=Exams,3=Attempts,4=Integrations
  const snack = useSnack();

  async function login(username: string, password: string) {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<{ access_token: string }>("/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role: "admin" }),
      });
      setJwt(data.access_token);
      setScreen("home");
      snack.setMsg("Logged in.");
    } catch (err: any) { snack.setErr(err.message); } finally { setBusy(false); }
  }
  function signOut() { setJwt(""); setScreen("login"); snack.setMsg("Signed out."); }

  const theme = useMemo(() => createTheme({
    palette: { mode: "light", primary: { main: "#3f51b5" } },
    shape: { borderRadius: 12 },
    components: { MuiPaper: { styleOverrides: { root: { borderRadius: 16 } } }, MuiButton: { defaultProps: { disableRipple: true } } },
  }), []);

  if (screen === "login") {
    return (<ThemeProvider theme={theme}><LoginScreen busy={busy} onLogin={login} /></ThemeProvider>);
  }

  return (
    <ThemeProvider theme={theme}>
      <Shell authed={true} onSignOut={signOut} title="Admin Console" right={
        <Tabs value={tab} onChange={(_, v) => setTab(v)} textColor="primary" indicatorColor="primary" sx={{ mr: 1 }}>
          <Tab icon={<SecurityIcon />} iconPosition="start" label="System" />
          <Tab icon={<ManageAccountsIcon />} iconPosition="start" label="Users" />
          <Tab icon={<LibraryBooksIcon />} iconPosition="start" label="Exams" />
          <Tab icon={<AssessmentIcon />} iconPosition="start" label="Attempts" />
          <Tab icon={<IntegrationInstructionsIcon />} iconPosition="start" label="Integrations" />
        </Tabs>
      }>
        {tab === 0 && <SystemPanel jwt={jwt} />}
        {tab === 1 && <UsersPanel jwt={jwt} />}
        {tab === 2 && <ExamsPanel jwt={jwt} />}
        {tab === 3 && <AttemptsPanel jwt={jwt} />}
        {tab === 4 && <IntegrationsPanel jwt={jwt} />}
        {snack.node}
      </Shell>
    </ThemeProvider>
  );
}
