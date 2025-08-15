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
import Grid from "@mui/material/Grid";
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
  started_at?: number;
  submitted_at?: number;
};

/** NEW: Course & Offering shapes (match /courses API) */
export type Course = {
  id: string;
  name: string;
};

export type Offering = {
  id: string;
  exam_id: string;
  start_at?: string | null; // RFC3339 string from server
  end_at?: string | null;   // RFC3339 string from server
  time_limit_sec?: number;
  max_attempts: number;
  visibility: "course" | "public" | "link";
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
/** UPDATED: adds Google Sign-On button; keeps existing local login intact */
function LoginScreen({ busy, onLogin }: { busy: boolean; onLogin: (u: string, p: string, r: "teacher" | "admin") => void; }) {
  const [username, setUsername] = useState("teacher");
  const [password, setPassword] = useState("teacher");
  const [role, setRole] = useState<"teacher" | "admin">("teacher");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onLogin(username, password, role);
  }

  /** NEW: Start Google OAuth flow (server will redirect to Google and back) */
  function googleSignIn() {
    // If your backend supports a "redirect" param, append it; otherwise plain login URL works.
    // Using a plain redirect keeps compatibility with the handlers you showed.
    window.location.href = `${API_BASE}/auth/google/login`;
  }

  return (
    <Shell authed={false} title="Sign in">
      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 7, lg: 6}}>
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
                <Divider>or</Divider>
                {/* NEW: Google SSO */}
                <Button variant="outlined" size="large" onClick={googleSignIn} disabled={busy}>
                  Sign in with Google
                </Button>
              </Stack>
            </Box>
          </Paper>
        </Grid>
        <Grid size={{xs:12,  md:5, lg:6}}>
          <Paper elevation={0} sx={{ p: 3, height: "100%" }}>
            <Typography variant="subtitle1" fontWeight={600}>What you can do</Typography>
            <Box component="ul" sx={{ mt: 1.5, pl: 3 }}>
              <li>Create or import exams (QTI)</li>
              <li>Export exams</li>
              <li>View attempts and scores</li>
              <li>Manage users (bulk CSV/JSON)</li>
            </Box>
          </Paper>
        </Grid>
      </Grid>
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
      const qs = query.trim() ? `?${new URLSearchParams({ q: query.trim() }).toString()}` : "";
      const data = await api<ExamSummary[]>(`/exams${qs}`, {
        headers: { Authorization: `Bearer ${jwt}` },
      });
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
        <Grid container spacing={1.5} alignItems="flex-end">
          <Grid size={{xs:12}}>
            <TextField label="Search exams" placeholder="title contains…" value={q} onChange={(e) => setQ(e.target.value)} fullWidth />
          </Grid>
          <Grid>
            <Button variant="outlined" onClick={() => fetchExams(q)} disabled={busy}>{busy ? "Searching…" : "Search"}</Button>
          </Grid>
          <Grid>
            <Button variant="text" onClick={() => { setQ(""); fetchExams(""); }} disabled={busy}>Reset</Button>
          </Grid>
          <Grid sx={{ display: { xs: 'none', sm: 'block' } }}>
            <Divider flexItem orientation="vertical" />
          </Grid>
          <Grid>
            <Button startIcon={<AddIcon />} variant="contained" disableElevation onClick={() => setOpenCreate(true)}>
              New Exam (JSON)
            </Button>
          </Grid>
          <Grid>
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
          </Grid>
          <Grid>
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
          </Grid>
        </Grid>
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

      {snack.node}
    </Stack>
  );
}

/* -------------------- Attempts Panel -------------------- */
function AttemptsPanel({ jwt }: { jwt: string; }) {
  const [filters, setFilters] = useState<{ exam_id: string; user_id: string; status: string; sort: string; pageSize: number; }>({
    exam_id: "",
    user_id: "",
    status: "",
    sort: "started_at desc",
    pageSize: 50,
  });
  const [page, setPage] = useState(0);
  const [busy, setBusy] = useState(false);
  const [list, setList] = useState<Attempt[]>([]);
  const [selected, setSelected] = useState<Attempt | null>(null);
  const snack = useSnack();

  const canPrev = page > 0;
  const canNext = list.length >= filters.pageSize;

  const load = useCallback(async () => {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const params = new URLSearchParams();
      if (filters.exam_id.trim()) params.set("exam_id", filters.exam_id.trim());
      if (filters.user_id.trim()) params.set("user_id", filters.user_id.trim());
      if (filters.status.trim()) params.set("status", filters.status.trim());
      if (filters.sort.trim()) params.set("sort", filters.sort.trim());
      params.set("limit", String(filters.pageSize));
      params.set("offset", String(page * filters.pageSize));

      const data = await api<Attempt[]>(`/attempts?${params.toString()}`, {
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setList(data);
    } catch (e: any) {
      snack.setErr(e.message);
    } finally {
      setBusy(false);
    }
  }, [jwt, filters, page]);

  useEffect(() => { load(); }, [load]);

  async function openAttemptDetails(id: string) {
    try {
      const data = await api<Attempt>(`/attempts/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${jwt}` } });
      setSelected(data);
      snack.setMsg(`Loaded attempt ${data.id}`);
    } catch (e: any) { snack.setErr(e.message); }
  }

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Grid container spacing={1.5} alignItems="flex-end">
          <Grid size={{xs:12, sm:6, md: "auto"}}>
            <TextField label="Exam ID (course)" value={filters.exam_id} onChange={(e) => setFilters((f) => ({ ...f, exam_id: e.target.value }))} fullWidth />
          </Grid>
          <Grid size={{xs:12, sm:6, md:"auto"}}>
            <TextField label="Student ID" value={filters.user_id} onChange={(e) => setFilters((f) => ({ ...f, user_id: e.target.value }))} fullWidth />
          </Grid>
          <Grid size={{xs:12, sm:6, md:"auto"}}>
            <FormControl sx={{ minWidth: 160 }}>
              <InputLabel id="status-filter">Status</InputLabel>
              <Select labelId="status-filter" label="Status" value={filters.status} onChange={(e) => setFilters((f) => ({ ...f, status: String(e.target.value) }))}>
                <MenuItem value="">(any)</MenuItem>
                <MenuItem value="in_progress">in_progress</MenuItem>
                <MenuItem value="submitted">submitted</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{xs:12, sm:6, md:"auto"}}>
            <FormControl sx={{ minWidth: 200 }}>
              <InputLabel id="sort-by">Sort</InputLabel>
              <Select labelId="sort-by" label="Sort" value={filters.sort} onChange={(e) => setFilters((f) => ({ ...f, sort: String(e.target.value) }))}>
                <MenuItem value="started_at desc">started_at desc</MenuItem>
                <MenuItem value="started_at asc">started_at asc</MenuItem>
                <MenuItem value="submitted_at desc">submitted_at desc</MenuItem>
                <MenuItem value="submitted_at asc">submitted_at asc</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{xs:12,  sm:6, md:"auto"}}>
            <FormControl sx={{ minWidth: 120 }}>
              <InputLabel id="page-size">Page size</InputLabel>
              <Select labelId="page-size" label="Page size" value={filters.pageSize} onChange={(e) => setFilters((f) => ({ ...f, pageSize: Number(e.target.value) }))}>
                <MenuItem value={20}>20</MenuItem>
                <MenuItem value={50}>50</MenuItem>
                <MenuItem value={100}>100</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{md:"auto"}}>
            <Button variant="contained" disableElevation onClick={() => { setPage(0); load(); }} disabled={busy}>{busy ? "Loading…" : "Refresh"}</Button>
          </Grid>
        </Grid>
      </Paper>

      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack spacing={1.25} sx={{ maxHeight: 480, overflowY: 'auto' }}>
          {list.length === 0 && !busy && (
            <Typography variant="body2" color="text.secondary">No attempts found.</Typography>
          )}
          {list.map((a) => (
            <Paper key={a.id} variant="outlined" sx={{ p: 1.5 }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'center' }}>
                <Box sx={{ flexGrow: 1 }}>
                  <Typography fontWeight={600}>Attempt <Box component="span" sx={{ fontFamily: 'monospace' }}>{a.id}</Box></Typography>
                  <Typography variant="caption" color="text.secondary">
                    Exam: <Box component="span" sx={{ fontFamily: 'monospace' }}>{a.exam_id}</Box> • Student: <Box component="span" sx={{ fontFamily: 'monospace' }}>{a.user_id}</Box> • Status: {a.status}
                    {typeof a.score === 'number' && <> • Score: {a.score}</>}
                    {a.started_at && <> • Started: {ms2date(a.started_at)}</>}
                    {a.submitted_at && <> • Submitted: {ms2date(a.submitted_at)}</>}
                  </Typography>
                </Box>
                <Button variant="text" onClick={() => openAttemptDetails(a.id)}>Open</Button>
              </Stack>
            </Paper>
          ))}
        </Stack>
        <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between" sx={{ mt: 1 }}>
          <Typography variant="caption" color="text.secondary">Loaded {list.length} item(s)</Typography>
          <Stack direction="row" spacing={1} alignItems="center">
            <Button variant="outlined" disabled={!canPrev || busy} onClick={() => { if (canPrev) { setPage((p) => p - 1); } }}>{busy ? "…" : "Prev"}</Button>
            <Typography variant="caption" color="text.secondary">Page {page + 1}</Typography>
            <Button variant="outlined" disabled={!canNext || busy} onClick={() => { if (canNext) { setPage((p) => p + 1); } }}>{busy ? "…" : "Next"}</Button>
          </Stack>
        </Stack>
      </Paper>

      {selected && (
        <Paper elevation={1} sx={{ p: 2.5 }}>
          <Stack spacing={1}>
            <Typography variant="h6" fontWeight={600}>Attempt {selected.id}</Typography>
            <Typography variant="body2" color="text.secondary">Exam: <Box component="span" sx={{ fontFamily: 'monospace' }}>{selected.exam_id}</Box> • User: {selected.user_id} • Status: {selected.status} {typeof selected.score === 'number' && (<Chip size="small" color="success" label={`Score: ${selected.score}`} sx={{ ml: 1 }} />)}</Typography>
            <Divider sx={{ my: 1 }} />
            <Typography variant="subtitle2">Responses (raw)</Typography>
            <Paper variant="outlined" sx={{ p: 2, bgcolor: 'grey.50', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace', whiteSpace: 'pre-wrap' }}>
              {JSON.stringify(selected.responses ?? {}, null, 2)}
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
      const qs = role ? `?${new URLSearchParams({ role }).toString()}` : "";
      const data = await api<Array<{ id: string; username: string; role: string }>>(
        `/users${qs}`,
        { headers: { Authorization: `Bearer ${jwt}` } }
      );
      setUsers(data);
    } catch (e: any) {
      snack.setErr(e.message);
    } finally {
      setBusy(false);
    }
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
        <Grid container spacing={1.5} alignItems="flex-end">
          <Grid size={{xs:12, sm:"auto"}}>
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
          <Grid size={{xs:12, sm:"auto"}}>
            <Button variant="outlined" onClick={fetchUsers} disabled={busy}>{busy ? 'Loading…' : 'Refresh'}</Button>
          </Grid>
          <Grid sx={{ display: { xs: 'none', sm: 'block' } }}>
            <Divider flexItem orientation="vertical" />
          </Grid>
          <Grid size={{xs:12, sm:"auto"}}>
            <Button component="label" startIcon={<UploadFileIcon />} variant="contained" disableElevation>
              Bulk Upload CSV/JSON
              <input type="file" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) bulkUpload(f); e.currentTarget.value = ""; }} />
            </Button>
          </Grid>
          <Grid sx={{ flexGrow: 1 }} />
          <Grid size={{xs:12, sm:"auto"}}>
            <Button onClick={() => setChangePwOpen(true)} startIcon={<ManageAccountsIcon />} variant="text">Change my password</Button>
          </Grid>
        </Grid>
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

/* -------------------- Courses Panel (NEW) -------------------- */
function CoursesPanel({ jwt }: { jwt: string }) {
  const snack = useSnack();
  const [busy, setBusy] = useState(false);

  const [courses, setCourses] = useState<Course[]>([]);
  const [selectedCourseId, setSelectedCourseId] = useState<string>("");

  const [offerings, setOfferings] = useState<Offering[]>([]);
  const [offerBusy, setOfferBusy] = useState(false);

  // create course
  const [newCourseName, setNewCourseName] = useState("");

  // add teachers / enroll students
  const [teacherIds, setTeacherIds] = useState("");
  const [teacherRole, setTeacherRole] = useState<"co" | "owner">("co");
  const [studentIds, setStudentIds] = useState("");
  const [studentStatus, setStudentStatus] = useState<"active" | "invited" | "dropped">("active");

  // create offering
  const [examList, setExamList] = useState<ExamSummary[]>([]);
  const [selExamId, setSelExamId] = useState<string>("");
  const [startAt, setStartAt] = useState<string>(""); // datetime-local
  const [endAt, setEndAt] = useState<string>("");
  const [timeLimitSec, setTimeLimitSec] = useState<string>("");
  const [maxAttempts, setMaxAttempts] = useState<string>("1");
  const [visibility, setVisibility] = useState<"course" | "public" | "link">("course");
  const [accessToken, setAccessToken] = useState<string>("");

  function fmtRFC(s?: string | null) {
    if (!s) return "";
    const d = new Date(s);
    if (isNaN(d.getTime())) return String(s);
    return d.toLocaleString();
  }

  function toUnixSeconds(local: string | undefined) {
    if (!local) return undefined;
    const ms = new Date(local).getTime();
    if (isNaN(ms)) return undefined;
    return Math.floor(ms / 1000);
  }

  const loadCourses = useCallback(async () => {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Course[]>("/courses", { headers: { Authorization: `Bearer ${jwt}` } });
      setCourses(data);
      if (data.length > 0 && !selectedCourseId) setSelectedCourseId(data[0].id);
    } catch (e: any) {
      snack.setErr(e.message);
    } finally {
      setBusy(false);
    }
  }, [jwt, selectedCourseId]);

  const loadOfferings = useCallback(async (courseId: string) => {
    if (!courseId) { setOfferings([]); return; }
    setOfferBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Offering[]>(`/courses/${encodeURIComponent(courseId)}/offerings`, {
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setOfferings(data);
    } catch (e: any) {
      snack.setErr(e.message);
    } finally {
      setOfferBusy(false);
    }
  }, [jwt]);

  const loadExams = useCallback(async () => {
    try {
      const list = await api<ExamSummary[]>("/exams", { headers: { Authorization: `Bearer ${jwt}` } });
      setExamList(list);
      if (list.length > 0 && !selExamId) setSelExamId(list[0].id);
    } catch (e: any) {
      // non-fatal
    }
  }, [jwt, selExamId]);

  useEffect(() => { loadCourses(); loadExams(); }, [loadCourses, loadExams]);
  useEffect(() => { if (selectedCourseId) loadOfferings(selectedCourseId); }, [selectedCourseId, loadOfferings]);

  async function createCourse() {
    if (!newCourseName.trim()) return;
    try {
      const res = await fetch(`${API_BASE}/courses/`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify({ name: newCourseName.trim() }),
      });
      if (!res.ok) throw new Error(await res.text());
      await res.json();
      setNewCourseName("");
      snack.setMsg("Course created");
      loadCourses();
    } catch (e: any) {
      snack.setErr(e.message);
    }
  }

  async function addTeachers() {
    if (!selectedCourseId) return;
    const ids = teacherIds.split(",").map(s => s.trim()).filter(Boolean);
    if (ids.length === 0) return;
    try {
      const res = await fetch(`${API_BASE}/courses/${encodeURIComponent(selectedCourseId)}/teachers`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify({ user_ids: ids, role: teacherRole }),
      });
      if (!res.ok) throw new Error(await res.text());
      setTeacherIds("");
      snack.setMsg("Co-teachers updated");
    } catch (e: any) { snack.setErr(e.message); }
  }

  async function enrollStudents() {
    if (!selectedCourseId) return;
    const ids = studentIds.split(",").map(s => s.trim()).filter(Boolean);
    if (ids.length === 0) return;
    try {
      const res = await fetch(`${API_BASE}/courses/${encodeURIComponent(selectedCourseId)}/students`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify({ user_ids: ids, status: studentStatus }),
      });
      if (!res.ok) throw new Error(await res.text());
      setStudentIds("");
      snack.setMsg("Students updated");
    } catch (e: any) { snack.setErr(e.message); }
  }

  async function createOffering() {
    if (!selectedCourseId) return;
    if (!selExamId.trim()) { snack.setErr("Select an exam"); return; }
    try {
      const payload: any = {
        exam_id: selExamId.trim(),
        max_attempts: Number(maxAttempts || "1"),
        visibility,
      };
      const start = toUnixSeconds(startAt || undefined);
      const end = toUnixSeconds(endAt || undefined);
      if (typeof start === "number") payload.start_at = start;
      if (typeof end === "number") payload.end_at = end;
      if (timeLimitSec.trim()) payload.time_limit_sec = Number(timeLimitSec.trim());
      if (visibility === "link" && accessToken.trim()) payload.access_token = accessToken.trim();

      const res = await fetch(`${API_BASE}/courses/${encodeURIComponent(selectedCourseId)}/offerings`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error(await res.text());
      await res.json();
      snack.setMsg("Offering created");
      setStartAt(""); setEndAt(""); setTimeLimitSec(""); setMaxAttempts("1"); setVisibility("course"); setAccessToken("");
      loadOfferings(selectedCourseId);
    } catch (e: any) { snack.setErr(e.message); }
  }

  return (
    <Stack spacing={3}>
      <Paper elevation={1} sx={{ p: 2.5 }}>
        <Stack direction={{ xs: "column", md: "row" }} spacing={2} alignItems={{ md: "flex-end" }}>
          <FormControl sx={{ minWidth: 260 }}>
            <InputLabel id="course-select">My Courses</InputLabel>
            <Select
              labelId="course-select"
              label="My Courses"
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
          <Button variant="outlined" onClick={loadCourses} disabled={busy}>{busy ? "…" : "Refresh"}</Button>
          <Box sx={{ flexGrow: 1 }} />
          <TextField
            label="New course name"
            value={newCourseName}
            onChange={(e) => setNewCourseName(e.target.value)}
            sx={{ minWidth: 240 }}
          />
          <Button variant="contained" disableElevation onClick={createCourse} disabled={!newCourseName.trim()}>
            Create Course
          </Button>
        </Stack>
      </Paper>

      <Grid container spacing={3}>
        {/* Left: enrollment & teachers */}
        <Grid size={{xs:12, md:6}}>
          <Paper elevation={1} sx={{ p: 2.5, height: "100%" }}>
            <Typography variant="h6" fontWeight={600}>Teachers</Typography>
            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "flex-end" }} sx={{ mt: 1.5 }}>
              <TextField
                label="User IDs (comma-separated)"
                value={teacherIds}
                onChange={(e) => setTeacherIds(e.target.value)}
                fullWidth
              />
              <FormControl sx={{ minWidth: 140 }}>
                <InputLabel id="teacher-role">Role</InputLabel>
                <Select labelId="teacher-role" label="Role" value={teacherRole} onChange={(e) => setTeacherRole(e.target.value as any)}>
                  <MenuItem value="co">co</MenuItem>
                  <MenuItem value="owner">owner</MenuItem>
                </Select>
              </FormControl>
              <Button variant="contained" disableElevation onClick={addTeachers} disabled={!selectedCourseId || !teacherIds.trim()}>
                Add / Update
              </Button>
            </Stack>

            <Divider sx={{ my: 2 }} />

            <Typography variant="h6" fontWeight={600}>Students</Typography>
            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "flex-end" }} sx={{ mt: 1.5 }}>
              <TextField
                label="User IDs (comma-separated)"
                value={studentIds}
                onChange={(e) => setStudentIds(e.target.value)}
                fullWidth
              />
              <FormControl sx={{ minWidth: 140 }}>
                <InputLabel id="student-status">Status</InputLabel>
                <Select labelId="student-status" label="Status" value={studentStatus} onChange={(e) => setStudentStatus(e.target.value as any)}>
                  <MenuItem value="active">active</MenuItem>
                  <MenuItem value="invited">invited</MenuItem>
                  <MenuItem value="dropped">dropped</MenuItem>
                </Select>
              </FormControl>
              <Button variant="contained" disableElevation onClick={enrollStudents} disabled={!selectedCourseId || !studentIds.trim()}>
                Enroll / Update
              </Button>
            </Stack>
          </Paper>
        </Grid>

        {/* Right: offerings */}
        <Grid size={{xs:12, md:6}}>
          <Paper elevation={1} sx={{ p: 2.5, height: "100%" }}>
            <Typography variant="h6" fontWeight={600}>Offerings</Typography>

            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "flex-end" }} sx={{ mt: 1.5 }}>
              <FormControl sx={{ minWidth: 220 }}>
                <InputLabel id="exam-select">Exam</InputLabel>
                <Select labelId="exam-select" label="Exam" value={selExamId} onChange={(e) => setSelExamId(String(e.target.value))}>
                  {examList.map(ex => (
                    <MenuItem key={ex.id} value={ex.id}>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Box component="span" sx={{ fontFamily: "monospace", fontSize: 12 }}>{ex.id}</Box>
                        <Box component="span">• {ex.title}</Box>
                      </Box>
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
              <TextField
                label="Start (local)"
                type="datetime-local"
                value={startAt}
                onChange={(e) => setStartAt(e.target.value)}
                InputLabelProps={{ shrink: true }}
              />
              <TextField
                label="End (local)"
                type="datetime-local"
                value={endAt}
                onChange={(e) => setEndAt(e.target.value)}
                InputLabelProps={{ shrink: true }}
              />
            </Stack>

            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "flex-end" }} sx={{ mt: 1.5 }}>
              <TextField
                label="Time limit (sec)"
                type="number"
                value={timeLimitSec}
                onChange={(e) => setTimeLimitSec(e.target.value)}
                sx={{ minWidth: 160 }}
              />
              <TextField
                label="Max attempts"
                type="number"
                value={maxAttempts}
                onChange={(e) => setMaxAttempts(e.target.value)}
                sx={{ minWidth: 160 }}
              />
              <FormControl sx={{ minWidth: 160 }}>
                <InputLabel id="vis-select">Visibility</InputLabel>
                <Select labelId="vis-select" label="Visibility" value={visibility} onChange={(e) => setVisibility(e.target.value as any)}>
                  <MenuItem value="course">course</MenuItem>
                  <MenuItem value="public">public</MenuItem>
                  <MenuItem value="link">link</MenuItem>
                </Select>
              </FormControl>
              <TextField
                label="Access token (for link)"
                value={accessToken}
                onChange={(e) => setAccessToken(e.target.value)}
                sx={{ minWidth: 220 }}
              />
              <Button variant="contained" disableElevation onClick={createOffering} disabled={!selectedCourseId || !selExamId}>
                Create Offering
              </Button>
            </Stack>

            <Divider sx={{ my: 2 }} />

            <Stack spacing={1.25} sx={{ maxHeight: 280, overflowY: "auto" }}>
              {offerings.length === 0 && !offerBusy && <Typography variant="body2" color="text.secondary">No offerings yet.</Typography>}
              {offerings.map(o => (
                <Paper key={o.id} variant="outlined" sx={{ p: 1.25 }}>
                  <Stack direction={{ xs: "column", sm: "row" }} spacing={1} alignItems={{ sm: "center" }}>
                    <Box sx={{ flexGrow: 1 }}>
                      <Typography fontWeight={600}>
                        Offering <Box component="span" sx={{ fontFamily: "monospace", fontSize: 12 }}>{o.id}</Box>
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        Exam: <Box component="span" sx={{ fontFamily: "monospace" }}>{o.exam_id}</Box>
                        {o.start_at && <> • Starts: {fmtRFC(o.start_at)}</>}
                        {o.end_at && <> • Ends: {fmtRFC(o.end_at)}</>}
                        {typeof o.time_limit_sec === "number" && <> • ⏱ {Math.round((o.time_limit_sec || 0)/60)} min</>}
                        <> • Attempts: {o.max_attempts}</>
                        <> • Visibility: {o.visibility}</>
                      </Typography>
                    </Box>
                  </Stack>
                </Paper>
              ))}
            </Stack>
          </Paper>
        </Grid>
      </Grid>

      {snack.node}
    </Stack>
  );
}

/* -------------------- Root App -------------------- */
export default function TeacherApp() {
  type Screen = "login" | "home";
  const [screen, setScreen] = useState<Screen>("login");
  const [jwt, setJwt] = useState("")
  ;
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState(0); // 0=Exams, 1=Courses, 2=Attempts, 3=Users
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
      try { localStorage.setItem("teacher_jwt", data.access_token); } catch {}
      setScreen("home");
      snack.setMsg("Logged in.");
    } catch (err: any) { snack.setErr(err.message); } finally { setBusy(false); }
  }

  /** NEW: Accept JWT returned by Google callback via query or hash, and persist */
  useEffect(() => {
    // Try to read from URL (both query and hash) or from localStorage
    let token = "";
    const url = new URL(window.location.href);

    // query params
    token = url.searchParams.get("access_token") || url.searchParams.get("jwt") || "";

    // hash (#access_token=...)
    if (!token && window.location.hash) {
      const hashParams = new URLSearchParams(window.location.hash.slice(1));
      token = hashParams.get("access_token") || hashParams.get("jwt") || "";
    }

    // localStorage
    if (!token) {
      try { token = localStorage.getItem("teacher_jwt") || ""; } catch {}
    }

    if (token) {
      setJwt(token);
      try { localStorage.setItem("teacher_jwt", token); } catch {}
      setScreen("home");

      // clean URL (remove token-bearing params/hash)
      url.searchParams.delete("access_token");
      url.searchParams.delete("jwt");
      window.history.replaceState({}, document.title, url.pathname + (url.search ? `?${url.searchParams.toString()}` : ""));
      if (window.location.hash) {
        window.history.replaceState({}, document.title, url.pathname + (url.search ? `?${url.searchParams.toString()}` : ""));
      }
    }
  }, []);

  function signOut() {
    setJwt("");
    try { localStorage.removeItem("teacher_jwt"); } catch {}
    setScreen("login");
    snack.setMsg("Signed out.");
  }

  const theme = useMemo(() => createTheme({
    palette: { mode: "light", primary: { main: "#3f51b5" } },
    shape: { borderRadius: 12 },

    typography: {
      button: {
        fontSize: "0.85rem",      // <- controls default button text size
        textTransform: "none",     // optional: keep case as typed
      },
    },

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
          <Tab icon={<LibraryBooksIcon />} iconPosition="start" label="Courses" />
          <Tab icon={<AssignmentIcon />} iconPosition="start" label="Attempts" />
          <Tab icon={<ManageAccountsIcon />} iconPosition="start" label="Users" />
        </Tabs>
      }>
        {tab === 0 && <ExamsPanel jwt={jwt} />}
        {tab === 1 && <CoursesPanel jwt={jwt} />}
        {tab === 2 && <AttemptsPanel jwt={jwt} />}
        {tab === 3 && <UsersPanel jwt={jwt} />}
        {snack.node}
      </Shell>
    </ThemeProvider>
  );
}
