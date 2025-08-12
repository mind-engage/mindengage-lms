import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
  Checkbox,
  FormControlLabel,
} from "@mui/material";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import LogoutIcon from "@mui/icons-material/Logout";
import UploadFileIcon from "@mui/icons-material/UploadFile";
import FileDownloadIcon from "@mui/icons-material/FileDownload";
import AddIcon from "@mui/icons-material/Add";
import ManageAccountsIcon from "@mui/icons-material/ManageAccounts";
import AssignmentIcon from "@mui/icons-material/Assignment";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";

const API_BASE = process.env.REACT_APP_API_BASE || "http://localhost:8080/api";

/* -------------------- Types -------------------- */
export type Exam = {
  id: string;
  title: string;
  time_limit_sec?: number;
  questions: Question[];
  profile?: string;
  policy?: any; // authoring convenience (maps to policy JSON)
};

export type Question = {
  id: string;
  type: "mcq_single" | "mcq_multi" | "true_false" | "short_word" | "numeric" | "essay";
  prompt_html?: string;
  prompt?: string;
  choices?: { id: string; label_html?: string }[];
  points?: number;
  answer_key?: string[]; // teacher view only
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
  // Some endpoints (204) have no body
  try {
    return await res.json();
  } catch {
    return undefined as unknown as T;
  }
}

function htmlify(s?: string) {
  return { __html: s || "" };
}

function ms2date(ms?: number) {
  if (!ms) return "";
  const d = new Date(ms * 1000);
  return d.toLocaleString();
}

/* -------------------- Shared UI bits -------------------- */
function useSnack() {
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  return {
    msg,
    err,
    setMsg,
    setErr,
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
          <Typography variant="h6" sx={{ fontWeight: 600 }}>MindEngage • Teacher</Typography>
          {title && (
            <Typography variant="body2" color="text.secondary" sx={{ ml: 2, display: { xs: "none", md: "block" } }}>{title}</Typography>
          )}
          <Box sx={{ flexGrow: 1 }} />
          {right}
          {authed && onSignOut && (
            <Tooltip title="Sign out">
              <IconButton onClick={onSignOut} color="primary" aria-label="sign out">
                <LogoutIcon />
              </IconButton>
            </Tooltip>
          )}
        </Toolbar>
      </AppBar>
      <Container maxWidth="lg" sx={{ py: 4 }}>{children}</Container>
    </>
  );
}

/* -------------------- Login -------------------- */
function LoginScreen({ busy, onLogin }: { busy: boolean; onLogin: (u: string, p: string, r: "teacher" | "admin") => void; }) {
  const [username, setUsername] = useState("teacher");
  const [password, setPassword] = useState("teacher");
  const [role, setRole] = useState<"teacher" | "admin">("teacher");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onLogin(username, password, role);
  }

  return (
    <Shell authed={false} title="Sign in">
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
        <Box sx={{ width: { xs: '100%', md: `${(7 / 12) * 100}%`, lg: '50%' } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="h5" fontWeight={600}>Teacher/Admin Sign in</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>Use test creds (username=password). Choose your role.</Typography>
            <Box component="form" onSubmit={submit} sx={{ mt: 2 }}>
              <Stack spacing={2}>
                <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} fullWidth />
                <TextField label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} fullWidth />
                <FormControl fullWidth>
                  <InputLabel id="role-lbl">Role</InputLabel>
                  <Select labelId="role-lbl" label="Role" value={role} onChange={(e) => setRole(e.target.value as any)}>
                    <MenuItem value="teacher">teacher</MenuItem>
                    <MenuItem value="admin">admin</MenuItem>
                  </Select>
                </FormControl>
                <Button type="submit" variant="contained" size="large" disableElevation disabled={busy}>{busy ? "…" : "Login"}</Button>
              </Stack>
            </Box>
          </Paper>
        </Box>
        <Box sx={{ width: { xs: '100%', md: `${(5 / 12) * 100}%`, lg: '50%' } }}>
          <Paper elevation={0} sx={{ p: 3, height: "100%" }}>
            <Typography variant="subtitle1" fontWeight={600}>What you can do</Typography>
            <Box component="ul" sx={{ mt: 1.5, pl: 3 }}>
              <li>Create or import exams (QTI)</li>
              <li>Export exams</li>
              <li>View attempts and scores</li>
              <li>Manage users (bulk CSV/JSON)</li>
            </Box>
          </Paper>
        </Box>
      </Stack>
    </Shell>
  );
}

/* -------------------- Exams Panel -------------------- */
function ExamsPanel({ jwt }: { jwt: string; }) {
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [list, setList] = useState<ExamSummary[]>([]);
  const [viewExam, setViewExam] = useState<Exam | null>(null);
  const [openCreate, setOpenCreate] = useState(false);
  const [createJson, setCreateJson] = useState<string>(`{
  "id": "exam-" ,
  "title": "New Exam",
  "profile": "stem.v1",
  "policy": {
    "sections": [ { "id": "main", "modules": [ { "id": "m1", "time_limit_sec": 3600 } ] } ],
    "navigation": { "allow_back": false, "module_locked": true }
  },
  "time_limit_sec": 3600,
  "questions": [
    { "id": "q1", "type": "mcq_single", "prompt_html": "<p>2+2=?</p>", "choices": [ {"id":"A","label_html":"3"}, {"id":"B","label_html":"4"}, {"id":"C","label_html":"5"}, {"id":"D","label_html":"6"} ], "points": 1, "answer_key": ["B"] }
  ]
}`);
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
    try {
      const data = await api<Exam>(`/exams/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${jwt}` } });
      setViewExam(data);
    } catch (e: any) { snack.setErr(e.message); }
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

  /** ---- NEW: shared normalizer & saver for JSON-based exam creation/import ---- */
  function normalizeExamPayload(parsed: any) {
    // Backend expects policy as raw JSON bytes; send as `policy_raw`
    const { policy, ...rest } = parsed || {};
    return policy ? { ...rest, policy_raw: policy } : rest;
  }

  async function saveExam(parsed: any, sourceLabel?: string) {
    const payload = normalizeExamPayload(parsed);
    const res = await fetch(`${API_BASE}/exams`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error(await res.text());
    await res.json();
    snack.setMsg(sourceLabel ? `Exam saved from ${sourceLabel}.` : "Exam saved.");
    fetchExams(q);
  }

  /** ---- UPDATED: uses shared saveExam ---- */
  async function createExamFromJSON() {
    try {
      const parsed = JSON.parse(createJson);
      await saveExam(parsed);
      setOpenCreate(false);
    } catch (e: any) {
      snack.setErr(e.message?.startsWith("Unexpected token") ? "Invalid JSON in editor." : e.message);
    }
  }

  /** ---- NEW: import a local .json file and call the same create endpoint ---- */
  async function importExamJSON(file: File) {
    try {
      const text = await file.text();
      const parsed = JSON.parse(text);
      await saveExam(parsed, file.name);
    } catch (e: any) {
      if (e instanceof SyntaxError) {
        snack.setErr(`Invalid JSON in ${file.name}: ${e.message}`);
      } else {
        snack.setErr(e.message);
      }
    }
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

          {/* Existing: open JSON editor */}
          <Button startIcon={<AddIcon />} variant="contained" disableElevation onClick={() => setOpenCreate(true)}>
            New Exam (JSON)
          </Button>

          {/* NEW: Upload a local JSON exam file */}
          <Button component="label" startIcon={<UploadFileIcon />} variant="outlined">
            Import JSON
            <input
              type="file"
              accept=".json,application/json"
              hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) importExamJSON(f);
                e.currentTarget.value = "";
              }}
            />
          </Button>

          {/* Existing: Import QTI zip */}
          <Button component="label" startIcon={<UploadFileIcon />} variant="outlined">
            Import QTI
            <input
              type="file"
              hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) importQTI(f);
                e.currentTarget.value = "";
              }}
            />
          </Button>
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack spacing={1.25} sx={{ maxHeight: 480, overflowY: 'auto' }}>
          {list.length === 0 && !busy && (
            <Typography variant="body2" color="text.secondary">No exams found.</Typography>
          )}
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
              </Stack>
            </Paper>
          ))}
        </Stack>
      </Paper>

      {/* Exam preview dialog (student-safe) */}
      <Dialog open={!!viewExam} onClose={() => setViewExam(null)} maxWidth="md" fullWidth>
        <DialogTitle>Preview: {viewExam?.title}</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={2}>
            {viewExam?.questions.map((q, i) => (
              <Paper key={q.id} variant="outlined" sx={{ p: 2 }}>
                <Typography variant="caption" color="text.secondary">Q{i+1} • {q.type}</Typography>
                <Box sx={{ mt: 1, '& p': { my: 0.5 } }} dangerouslySetInnerHTML={htmlify(q.prompt_html || q.prompt)} />
                {q.choices && q.choices.length > 0 && (
                  <Box sx={{ mt: 1 }}>
                    {q.choices.map((c) => (
                      <Box key={c.id} sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.5 }}>
                        <Checkbox disabled />
                        <span dangerouslySetInnerHTML={htmlify(c.label_html)} />
                      </Box>
                    ))}
                  </Box>
                )}
              </Paper>
            ))}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setViewExam(null)}>Close</Button>
        </DialogActions>
      </Dialog>

      {/* Create exam dialog */}
      <Dialog open={openCreate} onClose={() => setOpenCreate(false)} maxWidth="md" fullWidth>
        <DialogTitle>New Exam (raw JSON)</DialogTitle>
        <DialogContent dividers>
          <DialogContentText sx={{ mb: 1 }}>
            Paste an exam JSON matching your backend schema. Policy will be validated server-side by profile adapter.
          </DialogContentText>
          <TextField value={createJson} onChange={(e) => setCreateJson(e.target.value)} fullWidth multiline minRows={14} spellCheck={false} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenCreate(false)}>Cancel</Button>
          <Button variant="contained" disableElevation onClick={createExamFromJSON}>Save</Button>
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
  const snack = useSnack();

  async function loadAttempt(id: string) {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Attempt>(`/attempts/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${jwt}` } });
      setAttempt(data);
      if (data) snack.setMsg(`Loaded attempt ${data.id}`);
    } catch (e: any) { snack.setErr(e.message); } finally { setBusy(false); }
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

/* -------------------- Users Panel -------------------- */
function UsersPanel({ jwt }: { jwt: string; }) {
  const [role, setRole] = useState<string>("");
  const [users, setUsers] = useState<Array<{ id: string; username: string; role: string }>>([]);
  const [busy, setBusy] = useState(false);
  const [changePwOpen, setChangePwOpen] = useState(false);
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
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

  useEffect(() => { fetchUsers(); /* initial load */ // eslint-disable-next-line react-hooks/exhaustive-deps
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

  async function changePassword() {
    try {
      const res = await fetch(`${API_BASE}/users/change-password`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` }, body: JSON.stringify({ old_password: oldPw, new_password: newPw }) });
      if (!res.ok && res.status !== 204) throw new Error(await res.text());
      snack.setMsg('Password changed');
      setChangePwOpen(false);
      setOldPw(""); setNewPw("");
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
          <Box sx={{ flexGrow: 1 }} />
          <Button onClick={() => setChangePwOpen(true)} startIcon={<ManageAccountsIcon />} variant="text">Change my password</Button>
        </Stack>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack spacing={1.25} sx={{ maxHeight: 480, overflowY: 'auto' }}>
          {users.length === 0 && !busy && (
            <Typography variant="body2" color="text.secondary">No users found.</Typography>
          )}
          {users.map((u) => (
            <Paper key={u.id} variant="outlined" sx={{ p: 1.25 }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
                <Box sx={{ flexGrow: 1 }}>
                  <Typography fontWeight={600}>{u.username}</Typography>
                  <Typography variant="caption" color="text.secondary">ID: <Box component="span" sx={{ fontFamily: 'monospace' }}>{u.id}</Box> • Role: {u.role}</Typography>
                </Box>
              </Stack>
            </Paper>
          ))}
        </Stack>
      </Paper>

      {/* Change password */}
      <Dialog open={changePwOpen} onClose={() => setChangePwOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Change Password</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={2}>
            <TextField label="Old password" type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} fullWidth />
            <TextField label="New password" type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} fullWidth />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setChangePwOpen(false)}>Cancel</Button>
          <Button variant="contained" disableElevation onClick={changePassword} disabled={!oldPw || !newPw}>Save</Button>
        </DialogActions>
      </Dialog>

      {snack.node}
    </Stack>
  );
}

/* -------------------- Root App -------------------- */
export default function TeacherApp() {
  type Screen = "login" | "home";
  const [screen, setScreen] = useState<Screen>("login");
  const [jwt, setJwt] = useState(""
  );
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState(0); // 0=Exams, 1=Attempts, 2=Users
  const snack = useSnack();

  async function login(username: string, password: string, role: "teacher" | "admin") {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<{ access_token: string }>("/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role }),
      });
      setJwt(data.access_token);
      setScreen("home");
      snack.setMsg("Logged in.");
    } catch (err: any) { snack.setErr(err.message); } finally { setBusy(false); }
  }

  function signOut() {
    setJwt("");
    setScreen("login");
    snack.setMsg("Signed out.");
  }

  const theme = useMemo(() => createTheme({
    palette: { mode: "light", primary: { main: "#3f51b5" } },
    shape: { borderRadius: 12 },
    components: {
      MuiPaper: { styleOverrides: { root: { borderRadius: 16 } } },
      MuiButton: { defaultProps: { disableRipple: true } },
    },
  }), []);

  if (screen === "login") {
    return (
      <ThemeProvider theme={theme}>
        <LoginScreen busy={busy} onLogin={login} />
      </ThemeProvider>
    );
  }

  return (
    <ThemeProvider theme={theme}>
      <Shell authed={true} onSignOut={signOut} title="Dashboard" right={
        <Tabs value={tab} onChange={(_, v) => setTab(v)} textColor="primary" indicatorColor="primary" sx={{ mr: 1 }}>
          <Tab icon={<LibraryBooksIcon />} iconPosition="start" label="Exams" />
          <Tab icon={<AssignmentIcon />} iconPosition="start" label="Attempts" />
          <Tab icon={<ManageAccountsIcon />} iconPosition="start" label="Users" />
        </Tabs>
      }>
        {tab === 0 && <ExamsPanel jwt={jwt} />}
        {tab === 1 && <AttemptsPanel jwt={jwt} />}
        {tab === 2 && <UsersPanel jwt={jwt} />}
        {snack.node}
      </Shell>
    </ThemeProvider>
  );
}
