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
  FormGroup,
  FormControlLabel,
  Checkbox,
  Radio,
  RadioGroup,
  Tabs,
  Tab,
  Divider,
  Tooltip,
} from "@mui/material";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import LogoutIcon from "@mui/icons-material/Logout";

const API_BASE = process.env.REACT_APP_API_BASE || "http://localhost:8080/api";

/* -------------------- Types -------------------- */
export type Exam = {
  id: string;
  title: string;
  time_limit_sec?: number;
  questions: Question[];
  policy?: any;
};

export type Question = {
  id: string;
  type: "mcq_single" | "mcq_multi" | "true_false" | "short_word" | "numeric" | "essay";
  prompt_html?: string;
  prompt?: string;
  choices?: { id: string; label_html?: string }[];
  points?: number;
};

export type Attempt = {
  id: string;
  exam_id: string;
  user_id: string;
  status: "in_progress" | "submitted" | string;
  score?: number;
  responses?: Record<string, any>;
  started_at?: number;   // NEW
  submitted_at?: number; // NEW

  module_id?: string;
  module_index?: number;
  remaining_seconds?: number;
};

export type ExamSummary = {
  id: string;
  title: string;
  time_limit_sec?: number;
  created_at?: number;
  profile?: string;
};

/** NEW: course & offering types */
type Course = { id: string; name: string };
type Offering = {
  id: string;
  exam_id: string;
  start_at?: string | null; // RFC3339 from server
  end_at?: string | null;   // RFC3339 from server
  time_limit_sec?: number | null;
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
  if (res.status === 204) {
    return undefined as T;
  }
  return res.json();
}

function htmlify(s?: string) {
  return { __html: s || "" };
}

function formatTime(s?: number | null) {
  if (s == null) return "--:--";
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
}

function normType(t?: string): Question["type"] {
  return (t || "").replace(/-/g, "_") as Question["type"];
}

/* window helpers for offerings */
function parseIsoOrNull(s?: string | null): Date | null {
  if (!s) return null;
  const d = new Date(s);
  return isNaN(d.getTime()) ? null : d;
}
function offeringWindowState(o: Offering) {
  const now = new Date();
  const start = parseIsoOrNull(o.start_at);
  const end = parseIsoOrNull(o.end_at);
  if (start && now < start) return { open: false, reason: `Opens ${start.toLocaleString()}` };
  if (end && now > end)   return { open: false, reason: `Closed ${end.toLocaleString()}` };
  return { open: true };
}

function getJWTSubject(token: string): string | null {
  try {
    const part = token.split(".")[1];
    if (!part) return null;
    const b64 = part.replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const json = atob(padded);
    const obj = JSON.parse(json);
    // common claim names: sub, user_id, username
    return obj.sub || obj.user_id || obj.username || null;
  } catch {
    return null;
  }
}

/* -------------------- Memo bits -------------------- */
const TimerChip = React.memo(function TimerChip({ label }: { label?: string | null }) {
  return label ? <Chip label={`⏱ ${label}`} variant="outlined" sx={{ mr: 2 }} /> : null;
});

type QuestionCardProps = {
  q: Question;
  idx: number;
  value: any;
  onChange: (qid: string, val: any) => void;
  disabled?: boolean; // NEW
};

const QuestionCard = React.memo(function QuestionCard({ q, idx, value, onChange, disabled }: QuestionCardProps) {
  const qtype = normType(q.type);

  // Helper to short-circuit when disabled
  const handleChange = (qid: string, val: any) => {
    if (disabled) return;
    onChange(qid, val);
  };

  return (
    <Paper variant="outlined" sx={{ p: 2.5 }}>
      <Stack spacing={2}>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography variant="caption" color="text.secondary">
            Q{idx + 1} • {qtype.replace(/_/g, "-")}{q.points ? `  ( ${q.points} pt )` : ""}
          </Typography>
        </Stack>

        <Box sx={{ "& p": { my: 0.5 } }} dangerouslySetInnerHTML={htmlify(q.prompt_html || q.prompt)} />

      {qtype === "mcq_single" && (
        <RadioGroup value={value ?? ""} onChange={(e) => handleChange(q.id, e.target.value)}>
          {q.choices?.map((c) => (
            <FormControlLabel
              key={c.id}
              value={c.id}
              control={<Radio disabled={disabled} />}
              label={<span dangerouslySetInnerHTML={htmlify(c.label_html)} />}
              disabled={disabled} // ensure label also disabled
            />
          ))}
        </RadioGroup>
      )}

      {qtype === "true_false" && (
        <RadioGroup value={value ?? ""} onChange={(e) => handleChange(q.id, e.target.value)}>
          {(["true", "false"] as const).map((v) => (
            <FormControlLabel
              key={v}
              value={v}
              control={<Radio disabled={disabled} />}
              label={<span style={{ textTransform: "capitalize" }}>{v}</span>}
              disabled={disabled}
            />
          ))}
        </RadioGroup>
      )}

      {qtype === "mcq_multi" && (
        <FormGroup>
          {q.choices?.map((c) => {
            const arr: string[] = Array.isArray(value) ? value : [];
            const checked = arr.includes(c.id);
            return (
              <FormControlLabel
                key={c.id}
                control={
                  <Checkbox
                    disabled={disabled}
                    checked={checked}
                    onChange={(e) => {
                      const next = new Set(arr);
                      if (e.target.checked) next.add(c.id);
                      else next.delete(c.id);
                      handleChange(q.id, Array.from(next));
                    }}
                  />
                }
                label={<span dangerouslySetInnerHTML={htmlify(c.label_html)} />}
                disabled={disabled}
              />
            );
          })}
        </FormGroup>
      )}

      {qtype === "short_word" && (
        <TextField
          fullWidth
          placeholder="Your answer"
          value={value ?? ""}
          onChange={(e) => handleChange(q.id, e.target.value)}
          disabled={disabled}
          InputProps={{ readOnly: disabled }}
        />
      )}

      {qtype === "numeric" && (
        <TextField
          type="number"
          fullWidth
          placeholder="0"
          value={value ?? ""}
          onChange={(e) => handleChange(q.id, e.target.value)}
          disabled={disabled}
          InputProps={{ readOnly: disabled }}
        />
      )}

      {qtype === "essay" && (
        <TextField
          fullWidth
          multiline
          minRows={6}
          placeholder="Write your answer..."
          value={value ?? ""}
          onChange={(e) => handleChange(q.id, e.target.value)}
          disabled={disabled}
          InputProps={{ readOnly: disabled }}
        />
      )}
      </Stack>
    </Paper>
  );
}, (prev, next) => prev.q === next.q && prev.idx === next.idx && prev.value === next.value && prev.disabled === next.disabled);

/* -------------------------------------------------- */

/* -------------------- Shared Shell -------------------- */
function Shell({
  children,
  authed,
  onSignOut,
  attempt,
  progressPct,
  timer,
  title,
}: {
  children: React.ReactNode;
  authed: boolean;
  onSignOut?: () => void;
  attempt?: Attempt | null;
  progressPct?: number;
  timer?: string | null;
  title?: string;
}) {
  return (
    <>
      <AppBar position="sticky" color="inherit" elevation={1} sx={{ backdropFilter: "saturate(180%) blur(6px)", background: "rgba(255,255,255,0.9)" }}>
        <Toolbar>
          <Box sx={{ width: 10, height: 10, bgcolor: "primary.main", borderRadius: 2, mr: 1.5 }} />
          <Typography variant="h6" sx={{ fontWeight: 600 }}>MindEngage • Student</Typography>
          {title && (
            <Typography variant="body2" color="text.secondary" sx={{ ml: 2, display: { xs: "none", md: "block" } }}>{title}</Typography>
          )}
          <Box sx={{ flexGrow: 1 }} />
          {attempt && typeof progressPct === "number" && (
            <Stack direction="row" alignItems="center" spacing={1} sx={{ mr: 2, display: { xs: "none", sm: "flex" } }}>
              <Typography variant="body2" color="text.secondary">Progress</Typography>
              <Box sx={{ width: 160 }}>
                <LinearProgress variant="determinate" value={Math.round(progressPct)} />
              </Box>
              <Chip size="small" label={`${Math.round(progressPct)}%`} />
            </Stack>
          )}
          {attempt && <TimerChip label={timer} />}
          {authed && onSignOut && (
            <IconButton onClick={onSignOut} color="primary" aria-label="sign out">
              <LogoutIcon />
            </IconButton>
          )}
        </Toolbar>
      </AppBar>
      <Container maxWidth="lg" sx={{ py: 4 }}>{children}</Container>
    </>
  );
}

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
          <Alert severity="info" variant="filled" onClose={() => setMsg(null)}>{msg}</Alert>
        </Snackbar>
        <Snackbar open={!!err} autoHideDuration={4500} onClose={() => setErr(null)} anchorOrigin={{ vertical: "bottom", horizontal: "center" }}>
          <Alert severity="error" variant="filled" onClose={() => setErr(null)}>{err}</Alert>
        </Snackbar>
      </>
    ),
  } as const;
}

/* -------------------- Screen 1: Login -------------------- */
function LoginScreen({ busy, onLogin }: { busy: boolean; onLogin: (u: string, p: string) => void; }) {
  const [username, setUsername] = useState("student");
  const [password, setPassword] = useState("student");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onLogin(username, password);
  }

  function loginWithGoogle() {
    const redirectBack = window.location.href;
    window.location.href = `${API_BASE}/auth/google/login?redirect=${encodeURIComponent(redirectBack)}`;
  }

  return (
    <Shell authed={false} title="Sign in">
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
        <Box sx={{ width: { xs: '100%', md: `${(7 / 12) * 100}%`, lg: '50%' } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="h5" fontWeight={600}>Sign in</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>Use your test credentials or Google to continue.</Typography>
            <Box component="form" onSubmit={submit} sx={{ mt: 2 }}>
              <Stack spacing={2}>
                <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} fullWidth />
                <TextField label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} fullWidth />
                <Button type="submit" variant="contained" size="large" disableElevation disabled={busy}>{busy ? "…" : "Login"}</Button>
                <Button type="button" variant="outlined" size="large" onClick={loginWithGoogle} disabled={busy}>Sign in with Google</Button>
              </Stack>
            </Box>
          </Paper>
        </Box>
        <Box sx={{ width: { xs: '100%', md: `${(5 / 12) * 100}%`, lg: '50%' } }}>
          <Paper elevation={0} sx={{ p: 3, height: "100%" }}>
            <Typography variant="subtitle1" fontWeight={600}>Three-step flow</Typography>
            <Box component="ol" sx={{ mt: 1.5, pl: 3 }}>
              <li>Sign in</li>
              <li>Select exam or assignment</li>
              <li>Take the exam and submit</li>
            </Box>
          </Paper>
        </Box>
      </Stack>
    </Shell>
  );
}

/* -------------------- Screen 2: Select -------------------- */
function SelectScreen({
  jwt,
  onBack,
  onLoadExam,
}: {
  jwt: string;
  onBack: () => void;
  onLoadExam: (id: string, offering?: Offering) => void; // UPDATED: can pass offering
}) {
  const [tab, setTab] = useState(0); // 0=My Courses, 1=Search All
  const snack = useSnack();

  // ----- My Courses / Offerings -----
  const [courses, setCourses] = useState<Course[]>([]);
  const [selCourse, setSelCourse] = useState<string>("");
  const [offerings, setOfferings] = useState<Offering[]>([]);
  const [busyCourses, setBusyCourses] = useState(false);
  const [busyOfferings, setBusyOfferings] = useState(false);

  const loadCourses = useCallback(async () => {
    setBusyCourses(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Course[]>("/courses", { headers: { Authorization: `Bearer ${jwt}` } });
      setCourses(data);
      if (data.length && !selCourse) setSelCourse(data[0].id);
    } catch (e: any) { snack.setErr(e.message); } finally { setBusyCourses(false); }
  }, [jwt, selCourse]);

  const loadOfferings = useCallback(async (courseId: string) => {
    if (!courseId) { setOfferings([]); return; }
    setBusyOfferings(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Offering[]>(`/courses/${encodeURIComponent(courseId)}/offerings`, {
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setOfferings(data);
    } catch (e: any) { snack.setErr(e.message); } finally { setBusyOfferings(false); }
  }, [jwt]);

  useEffect(() => { loadCourses(); }, [loadCourses]);
  useEffect(() => { if (selCourse) loadOfferings(selCourse); }, [selCourse, loadOfferings]);

  // ----- All Exams (legacy path) -----
  const [examId, setExamId] = useState("");
  const [q, setQ] = useState("");
  const [busySearch, setBusySearch] = useState(false);
  const [list, setList] = useState<ExamSummary[]>([]);

  const fetchExams = useCallback(async (query: string) => {
    setBusySearch(true); snack.setErr(null); snack.setMsg(null);
    try {
      const qs = query.trim() ? `?q=${encodeURIComponent(query.trim())}` : "";
      const data = await api<ExamSummary[]>(
        `/exams${qs}`,
        { headers: { Authorization: `Bearer ${jwt}` } }
      );
      setList(data);
    } catch (err: any) {
      snack.setErr(err.message);
    } finally {
      setBusySearch(false);
    }
  }, [jwt]);

  useEffect(() => { fetchExams(""); }, [fetchExams]);

  function openByOffering(o: Offering) {
    onLoadExam(o.exam_id, o);
  }
  function openByExamId(id: string) {
    onLoadExam(id);
  }

  return (
    <Shell authed={true} title="Select exam or assignment" onSignOut={onBack}>
      <Paper elevation={1} sx={{ p: 1, mb: 2 }}>
        <Tabs value={tab} onChange={(_, v) => setTab(v)} textColor="primary" indicatorColor="primary">
          <Tab label="My Courses" />
          <Tab label="Search All Exams" />
        </Tabs>
      </Paper>

      {tab === 0 && (
        <Stack spacing={3}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "flex-end" }}>
              <FormControl sx={{ minWidth: 240 }}>
                <InputLabel id="course-select">Course</InputLabel>
                <Select
                  labelId="course-select"
                  label="Course"
                  value={selCourse}
                  onChange={(e) => setSelCourse(String(e.target.value))}
                >
                  {courses.map(c => (
                    <MenuItem key={c.id} value={c.id}>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Box component="span" sx={{ fontFamily: "monospace", fontSize: 12 }}>{c.id}</Box>
                        <Box component="span">• {c.name}</Box>
                      </Box>
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
              <Button variant="outlined" onClick={loadCourses} disabled={busyCourses}>{busyCourses ? "…" : "Refresh"}</Button>
            </Stack>
          </Paper>

          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="subtitle1" fontWeight={600}>Offerings</Typography>
            <Divider sx={{ my: 1 }} />
            <Stack spacing={1.25} sx={{ maxHeight: 420, overflowY: "auto" }}>
              {offerings.length === 0 && !busyOfferings && (
                <Typography variant="body2" color="text.secondary">No offerings for this course.</Typography>
              )}
              {offerings.map(o => {
                const win = offeringWindowState(o);
                return (
                  <Paper key={o.id} variant="outlined" sx={{ p: 1.5 }}>
                    <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "center" }}>
                      <Box sx={{ flexGrow: 1 }}>
                        <Typography fontWeight={600}>
                          <Box component="span" sx={{ fontFamily: "monospace", fontSize: 12 }}>{o.id}</Box>
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          Exam: <Box component="span" sx={{ fontFamily: "monospace" }}>{o.exam_id}</Box>
                          {o.start_at && <> • Starts: {new Date(o.start_at).toLocaleString()}</>}
                          {o.end_at && <> • Ends: {new Date(o.end_at).toLocaleString()}</>}
                          {typeof o.time_limit_sec === "number" && <> • ⏱ {Math.round((o.time_limit_sec || 0) / 60)} min</>}
                          <> • Attempts: {o.max_attempts}</>
                          <> • Visibility: {o.visibility}</>
                        </Typography>
                      </Box>
                      <Tooltip title={win.open ? "" : (win.reason || "Not available")}>
                        <span>
                          <Button variant="contained" disableElevation onClick={() => openByOffering(o)} disabled={!win.open}>
                            Open
                          </Button>
                        </span>
                      </Tooltip>
                    </Stack>
                  </Paper>
                );
              })}
            </Stack>
          </Paper>
        </Stack>
      )}

      {tab === 1 && (
        <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
          {/* Manual ID entry */}
          <Box sx={{ width: { xs: '100%', md: `${(5 / 12) * 100}%` } }}>
            <Paper elevation={1} sx={{ p: 3 }}>
              <Typography variant="h6" fontWeight={600}>Enter Exam ID</Typography>
              <Stack direction={{ xs: "column", sm: "row" }} spacing={2} alignItems={{ sm: "flex-end" }} sx={{ mt: 2 }}>
                <TextField label="Exam ID" value={examId} onChange={(e) => setExamId(e.target.value)} fullWidth />
                <Button variant="contained" onClick={() => openByExamId(examId)} disableElevation disabled={!examId.trim()}>
                  Load
                </Button>
              </Stack>
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>
                Or pick one from the list →
              </Typography>
            </Paper>
          </Box>

          {/* Available exams */}
          <Box sx={{ width: { xs: '100%', md: `${(7 / 12) * 100}%` } }}>
            <Paper elevation={1} sx={{ p: 3 }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-end' }}>
                <Box sx={{ flexGrow: 1 }}>
                  <TextField
                    label="Search exams"
                    placeholder="title contains…"
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    fullWidth
                  />
                </Box>
                <Button variant="outlined" onClick={() => fetchExams(q)} disabled={busySearch}>
                  {busySearch ? "Searching…" : "Search"}
                </Button>
                <Button variant="text" onClick={() => { setQ(""); fetchExams(""); }} disabled={busySearch}>
                  Reset
                </Button>
              </Stack>

              <Stack spacing={1.25} sx={{ mt: 2, maxHeight: 420, overflowY: 'auto' }}>
                {list.length === 0 && !busySearch && (
                  <Typography variant="body2" color="text.secondary">No exams found.</Typography>
                )}
                {list.map((e) => (
                  <Paper key={e.id} variant="outlined" sx={{ p: 1.5 }}>
                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'center' }}>
                      <Box sx={{ flexGrow: 1 }}>
                        <Typography fontWeight={600}>{e.title}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          <Box component="span" sx={{ fontFamily: "monospace" }}>{e.id}</Box>
                          {typeof e.time_limit_sec === "number" && (
                            <> • ⏱ {Math.round((e.time_limit_sec || 0) / 60)} min</>
                          )}
                          {e.profile && <> • {e.profile}</>}
                        </Typography>
                      </Box>
                      <Button variant="outlined" onClick={() => openByExamId(e.id)}>Open</Button>
                    </Stack>
                  </Paper>
                ))}
              </Stack>
            </Paper>
          </Box>
        </Stack>
      )}
      {snack.node}
    </Shell>
  );
}

/* -------------------- Screen 3: Exam -------------------- */
/* -------------------- Screen 3: Exam -------------------- */
function ExamScreen({ jwt, exam, offering, onExit }: { jwt: string; exam: Exam; offering?: Offering; onExit: () => void; }) {
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const [responses, setResponses] = useState<Record<string, any>>({});
  const [busy, setBusy] = useState(false);
  const [currentQ, setCurrentQ] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [secondsLeft, setSecondsLeft] = useState<number | null>(null);
  const timerRef = useRef<number | null>(null);
  const autosaveTRef = useRef<number | null>(null);
  const snack = useSnack();

  const moduleLocked = !!exam?.policy?.navigation?.module_locked;
  const allowBack = !!exam?.policy?.navigation?.allow_back; // NEW

  const [showSubmitted, setShowSubmitted] = useState(false); // NEW
  const isLocked = attempt?.status === "submitted"; // NEW

  // Stable updater so memoized children don't re-render unnecessarily
  const updateResponse = useCallback((qid: string, val: any) => {
    if (isLocked) return; // NEW
    setResponses((r) => {
      if (r[qid] === val) return r;
      return { ...r, [qid]: val };
    });
  }, [isLocked]); // UPDATED

  const timeLimit = offering?.time_limit_sec ?? exam.time_limit_sec ?? null;

  const total = exam?.questions.length ?? 0;
  const answered = useMemo(() => {
    if (!exam) return 0;
    return exam.questions.reduce((n, q) => {
      const v = responses[q.id];
      const has = !(v == null || v === "" || (Array.isArray(v) && v.length === 0));
      return n + (has ? 1 : 0);
    }, 0);
  }, [exam, responses]);
  const progressPct = total ? (answered / total) * 100 : 0;

  // ---- Module awareness (NEW) ----
  const moduleOrder = useMemo(() => {
    const secs = exam?.policy?.sections;
    if (!Array.isArray(secs)) return [];
    const ids: string[] = [];
    secs.forEach((s: any) => (s?.modules || []).forEach((m: any) => { if (m?.id) ids.push(m.id); }));
    return ids;
  }, [exam?.policy]);

  const qIdxByModule = useMemo(() => {
    const map = new Map<string, number[]>();
    (exam?.questions || []).forEach((q, i) => {
      const mid = (q as any)?.module_id || "__all__";
      if (!map.has(mid)) map.set(mid, []);
      map.get(mid)!.push(i);
    });
    return map;
  }, [exam?.questions]);

  const currentModuleId = useMemo(() => {
    if (attempt?.module_id) return attempt.module_id;
    if (typeof attempt?.module_index === "number" && moduleOrder[attempt.module_index]) return moduleOrder[attempt.module_index];
    return (exam?.questions?.[currentQ] as any)?.module_id || null;
  }, [attempt?.module_id, attempt?.module_index, moduleOrder, exam?.questions, currentQ]);

  const moduleIndices = useMemo(() => {
    if (currentModuleId) return qIdxByModule.get(currentModuleId) || [];
    return (exam?.questions || []).map((_, i) => i);
  }, [currentModuleId, qIdxByModule, exam?.questions]);

  const firstIdx = moduleIndices.length ? moduleIndices[0] : 0;
  const lastIdx = moduleIndices.length ? moduleIndices[moduleIndices.length - 1] : Math.max(0, (exam?.questions?.length || 1) - 1);

  // Keep currentQ inside the current module window
  useEffect(() => {
    if (currentQ < firstIdx || currentQ > lastIdx) setCurrentQ(firstIdx);
  }, [currentModuleId, firstIdx, lastIdx]); // NEW

  const isPrevDisabled = isLocked || !allowBack || currentQ <= firstIdx; // NEW
  const isNextDisabled = isLocked || currentQ >= lastIdx; // NEW

  // actions
  async function startAttempt() {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      // derive user from JWT (backend currently needs explicit user_id)
      const userId = getJWTSubject(jwt) || "student";
  
      const payload: any = { exam_id: exam.id, user_id: userId };
      // harmless for current backend (ignored if not used)
      if (offering?.id) payload.offering_id = offering.id;
  
      const data = await api<Attempt>("/attempts", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify(payload),
      });
      setAttempt(data);
      snack.setMsg(`Attempt ${data.id} started.`);
      if (timeLimit) setSecondsLeft(timeLimit);
    } catch (err: any) {
      snack.setErr(err.message);
    } finally { setBusy(false); }
  }

  async function saveResponses(manual = false) {
    if (!attempt) return;
    if (isLocked) { // NEW
      if (manual) snack.setMsg("Already submitted.");
      return;
    }
    try {
      const res = await fetch(`${API_BASE}/attempts/${attempt.id}/responses`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify(responses),
      });
      if (res.status === 409) { // NEW
        setAttempt((a) => (a ? { ...a, status: "submitted" } : a));
        if (manual) snack.setMsg("Already submitted.");
        return;
      }
      if (!res.ok) throw new Error(await res.text());
      if (manual) snack.setMsg("Responses saved.");
    } catch (err: any) {
      snack.setErr(err.message);
    }
  }

  async function submitAttempt() {
    if (!attempt || isLocked) { // NEW
      setShowSubmitted(true);
      return;
    }
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Attempt>(`/attempts/${attempt.id}/submit`, {
        method: "POST",
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setAttempt(data);
      setSecondsLeft(0); // NEW
      if (timerRef.current) window.clearInterval(timerRef.current!); // NEW
      if (autosaveTRef.current) window.clearTimeout(autosaveTRef.current!); // NEW
      setShowSubmitted(true); // NEW
      snack.setMsg(`Submitted. Score: ${data.score ?? 0}`);
    } catch (err: any) {
      snack.setErr(err.message);
    } finally { setBusy(false); }
  }

  async function uploadAsset(file: File) {
    if (!attempt) { snack.setErr("Start an attempt first."); return; }
    setUploading(true); snack.setErr(null); snack.setMsg(null);
    try {
      const form = new FormData();
      form.append("file", file);
      const res = await fetch(`${API_BASE}/assets/${attempt.id}`, {
        method: "POST",
        headers: { Authorization: `Bearer ${jwt}` },
        body: form,
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      snack.setMsg(`Uploaded: ${data.key}`);
    } catch (err: any) {
      snack.setErr(err.message);
    } finally { setUploading(false); }
  }

  async function nextModule() {
    if (!attempt) return;
  
    // Save before advancing
    await saveResponses(false);
  
    try {
      const res = await fetch(`${API_BASE}/attempts/${attempt.id}/next-module`, {
        method: "POST",
        headers: { Authorization: `Bearer ${jwt}` },
      });
  
      if (!res.ok) {
        const t = await res.text();
        // 409/400 is a common “not eligible yet” response — surface it
        throw new Error(t || `Advance blocked (${res.status})`);
      }
  
      // If server returns JSON with remaining_seconds, use it
      try {
        const data = await res.json();
        if (typeof data?.remaining_seconds === "number") {
          setSecondsLeft(data.remaining_seconds);
        }
      } catch {
        // no body (204) is fine
      }
  
      // UX reset for the new module
      setCurrentQ(0);
      snack.setMsg("Moved to next module.");
    } catch (e: any) {
      snack.setErr(e?.message || "Could not advance to the next module. You may need to complete current requirements first.");
    }
  }

  const totalModules = useMemo(() => {
    const secs = exam?.policy?.sections;
    if (!Array.isArray(secs)) return 0;
    return secs.reduce((n: number, s: any) => n + (Array.isArray(s?.modules) ? s.modules.length : 0), 0);
  }, [exam?.policy]);

  const canAdvanceModule = useMemo(() => {
    if (!attempt) return false;
    if (typeof attempt?.module_index === "number") {
      return totalModules > 0 && attempt.module_index < totalModules - 1;
    }
    return totalModules > 1;
  }, [attempt, totalModules]);

  // autosave (debounced, no per-keystroke stringify)
  useEffect(() => {
    if (!attempt || isLocked) return; // UPDATED
    if (autosaveTRef.current) window.clearTimeout(autosaveTRef.current);
    autosaveTRef.current = window.setTimeout(() => { saveResponses(false); }, 1000) as unknown as number;
    return () => { if (autosaveTRef.current) window.clearTimeout(autosaveTRef.current); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [responses, attempt?.id, isLocked]); // UPDATED

  // timer
  useEffect(() => {
    if (secondsLeft == null || !attempt || isLocked) return; // UPDATED
    if (timerRef.current) window.clearInterval(timerRef.current);
    timerRef.current = window.setInterval(() => {
      setSecondsLeft((prev) => {
        if (prev == null) return prev;
        if (prev <= 1) {
          window.clearInterval(timerRef.current!);
          submitAttempt();
          return 0;
        }
        return prev - 1;
      });
    }, 1000) as unknown as number;
    return () => { if (timerRef.current) window.clearInterval(timerRef.current); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [attempt?.id, secondsLeft !== null, isLocked]); // UPDATED

  useEffect(() => {
    if (!jwt || !exam?.id) return;
    (async () => {
      try {
        const userId = getJWTSubject(jwt) || "";
        const params = new URLSearchParams({
          exam_id: exam.id,
          user_id: userId,
          status: "in_progress",
          limit: "1",
          offset: "0",
        });
        const list = await api<Attempt[]>(`/attempts?${params}`, { headers: { Authorization: `Bearer ${jwt}` } });
        if (list[0]) {
          const a = await api<Attempt>(`/attempts/${encodeURIComponent(list[0].id)}`, { headers: { Authorization: `Bearer ${jwt}` } });
          setAttempt(a);
          setResponses(a.responses ?? {});
          // server-synced timer
          const tl = (offering?.time_limit_sec ?? exam.time_limit_sec) || 0;
          if (tl && a.started_at) {
            const elapsed = Math.floor(Date.now()/1000) - a.started_at;
            setSecondsLeft(Math.max(0, tl - elapsed));
          }
        }
      } catch {/* ignore */}
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jwt, exam?.id]);
  
  useEffect(() => { // NEW: lock UI and stop timers when submitted
    if (attempt?.status === "submitted") {
      setShowSubmitted(true);
      if (timerRef.current) window.clearInterval(timerRef.current);
      if (autosaveTRef.current) window.clearTimeout(autosaveTRef.current);
    }
  }, [attempt?.status]);

  useEffect(() => { // NEW: leave-page protection while in progress
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (attempt && !isLocked) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [attempt?.id, isLocked]);

  const isEssayQ = React.useMemo(() => {
    const q = exam?.questions?.[currentQ];
    return q ? normType(q.type) === "essay" : false;
  }, [exam, currentQ]);

  return (
    <Shell authed={true} onSignOut={onExit} attempt={attempt} progressPct={progressPct} timer={formatTime(secondsLeft)} title={exam.title}>
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3}>
        {/* Left rail */}
        <Box sx={{ width: { xs: '100%', lg: '25%' }, display: { xs: "none", lg: "block" } }}>
          <Stack spacing={2} sx={{ position: "sticky", top: 88 }}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Stack direction="row" alignItems="center" justifyContent="space-between">
                <Typography variant="body2">
                  Time Limit: {Math.round(((timeLimit || 0)) / 60)} min
                </Typography>

                {attempt && (
                  <Chip
                    size="small"
                    label={`Left: ${formatTime(secondsLeft)}`}
                    color={secondsLeft != null
                      ? secondsLeft <= 60 ? "error"
                        : secondsLeft <= 5 * 60 ? "warning"
                        : "default"
                      : "default"}
                    variant="outlined"
                  />
                )}
              </Stack>

              {attempt && timeLimit ? (
                <LinearProgress
                  sx={{ mt: 1 }}
                  variant="determinate"
                  value={
                    Math.min(
                      100,
                      Math.max(
                        0,
                        (((timeLimit || 0) - (secondsLeft || 0)) / (timeLimit || 1)) * 100
                      )
                    )
                  }
                />
              ) : null}
            </Paper>

            {exam && attempt && (
              <Paper
                variant="outlined"
                sx={{
                  p: 2,
                  maxHeight: 'calc(100vh - 88px - 24px - 120px)',
                  overflowY: 'auto',
                  overscrollBehavior: 'contain',
                }}
              >
                <Typography fontWeight={600} gutterBottom>Questions</Typography>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', mx: -0.5 }}>
                  {exam.questions.map((q, idx) => {
                    const r = responses[q.id];
                    const done = r != null && r !== "" && (!Array.isArray(r) || r.length > 0);

                    const notInModule = idx < firstIdx || idx > lastIdx; // NEW
                    const disabledBtn = isLocked || notInModule || (!allowBack && idx < currentQ); // NEW

                    return (
                      <Box key={q.id} sx={{ width: '25%', p: 0.5 }}>
                        <Button
                          fullWidth
                          size="small"
                          variant={currentQ === idx ? "contained" : (done ? "outlined" : "text")}
                          onClick={() => setCurrentQ(idx)}
                          disabled={disabledBtn}
                        >
                          {idx + 1}
                        </Button>
                      </Box>
                    );
                  })}
                </Box>
              </Paper>
            )}
          </Stack>
        </Box>

        {/* Right content */}
        <Box sx={{ width: { xs: '100%', lg: '75%' } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={2}>
              <Box>
                <Typography variant="h6" fontWeight={600}>{exam.title}</Typography>
                {timeLimit ? (
                  <Typography variant="caption" color="text.secondary">Time limit: {Math.round((timeLimit || 0) / 60)} min</Typography>
                ) : null}
              </Box>
              {!attempt ? (
                <Button onClick={startAttempt} variant="contained" disableElevation>{busy ? "Starting…" : "Start Attempt"}</Button>
              ) : (
                <Typography variant="body2">Attempt: <Box component="span" sx={{ fontFamily: "monospace" }}>{attempt.id}</Box> • {attempt.status}</Typography>
              )}
            </Stack>

            {attempt && (
              <Box sx={{ mt: 3 }}>
                {/* Mobile question nav */}
                <Box sx={{ display: { lg: "none" }, mb: 2 }}>
                  <FormControl fullWidth>
                    <InputLabel id="jump-q">Jump to question</InputLabel>
                    <Select
                      labelId="jump-q"
                      label="Jump to question"
                      value={currentQ}
                      onChange={(e) => {
                        const v = Number(e.target.value);
                        // Guard: only forward jumps when allow_back is false, and only inside module
                        if (isLocked) return;
                        if ((v < firstIdx || v > lastIdx)) return;
                        if (!allowBack && v < currentQ) return;
                        setCurrentQ(v);
                      }}
                      disabled={isLocked}
                    >
                      {exam.questions.map((q, idx) => {
                        const notInModule = idx < firstIdx || idx > lastIdx;
                        const itemDisabled = isLocked || notInModule || (!allowBack && idx < currentQ);
                        return (
                          <MenuItem key={q.id} value={idx} disabled={itemDisabled}>Q{idx + 1}</MenuItem>
                        );
                      })}
                    </Select>
                  </FormControl>
                </Box>

                <QuestionCard
                  key={exam.questions[currentQ].id}
                  q={exam.questions[currentQ]}
                  idx={currentQ}
                  value={responses[exam.questions[currentQ].id]}
                  onChange={updateResponse}
                  disabled={isLocked} // NEW
                />

                <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mt: 2 }}>
                  <Button variant="outlined" onClick={() => setCurrentQ((i) => Math.max(firstIdx, i - 1))} disabled={isPrevDisabled}>← Prev</Button>
                  <Button variant="outlined" onClick={() => setCurrentQ((i) => Math.min(lastIdx, i + 1))} disabled={isNextDisabled}>Next →</Button>
                  <Box sx={{ flexGrow: 1 }} />
                  <Typography variant="caption" color="text.secondary">Answered {answered} of {total}</Typography>
                </Stack>
              </Box>
            )}
          </Paper>

          {/* Sticky actions */}
          {attempt && (
            <Paper elevation={3} sx={{ position: "sticky", bottom: 0, mt: 3, p: 1.5, borderRadius: 2 }}>
              <Stack direction="row" alignItems="center" spacing={1}>
                <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>Autosaves as you type.</Typography>
                <Button onClick={() => saveResponses(true)} variant="outlined" disabled={isLocked}>Save now</Button>
                <Button onClick={nextModule} variant="outlined" disabled={!canAdvanceModule || isLocked}>Next Module</Button>
                <Button component="label" variant="outlined" disabled={!isEssayQ || uploading || isLocked}>
                  {uploading ? "Uploading…" : "Upload scan"}
                  <input type="file" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) uploadAsset(f); }} />
                </Button>
                <Box sx={{ flexGrow: 1 }} />
                <Button onClick={() => setConfirmOpen(true)} variant="contained" disableElevation disabled={isLocked}>Submit</Button>
                {attempt?.score !== undefined && (
                  <Chip color="success" label={`Score: ${attempt.score}`} sx={{ ml: 1 }} />
                )}
                {isLocked && attempt?.submitted_at && (
                  <Chip label={`Submitted ${new Date(attempt.submitted_at*1000).toLocaleString()}`} sx={{ ml: 1 }} />
                )}
              </Stack>
            </Paper>
          )}
        </Box>
      </Stack>

      {/* Confirm submit */}
      <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)}>
        <DialogTitle>Submit attempt?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Submit now? You won’t be able to edit after submitting.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmOpen(false)}>Cancel</Button>
          <Button onClick={() => { setConfirmOpen(false); submitAttempt(); }} variant="contained" disableElevation>Yes, submit</Button>
        </DialogActions>
      </Dialog>

      {/* Submitted dialog (NEW) */}
      <Dialog open={showSubmitted} onClose={() => setShowSubmitted(false)}>
        <DialogTitle>Attempt submitted</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {typeof attempt?.score === "number" ? `Your score: ${attempt.score}` : "Your attempt was submitted."}
            {attempt?.submitted_at ? ` • ${new Date(attempt.submitted_at*1000).toLocaleString()}` : ""}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={onExit} variant="contained" disableElevation>Back to exams</Button>
        </DialogActions>
      </Dialog>

      {snack.node}
    </Shell>
  );
}



/* -------------------- Root App -------------------- */
export default function StudentApp() {
  type Screen = "login" | "select" | "exam";
  const [screen, setScreen] = useState<Screen>("login");
  const [jwt, setJwt] = useState("");
  const [busy, setBusy] = useState(false);
  const [loadedExam, setLoadedExam] = useState<Exam | null>(null);
  const [loadedOffering, setLoadedOffering] = useState<Offering | undefined>(undefined); // NEW
  const snack = useSnack();

  // Capture JWT from URL (for Google callback redirects): ?access_token=... or #access_token=...
  useEffect(() => {
    try {
      const url = new URL(window.location.href);
      let t = url.searchParams.get("access_token") || url.searchParams.get("t");
      if (!t && url.hash && url.hash.startsWith("#")) {
        const h = new URLSearchParams(url.hash.slice(1));
        t = h.get("access_token") || h.get("t");
      }
      if (t) {
        setJwt(t);
        setScreen("select");
        url.searchParams.delete("access_token");
        url.searchParams.delete("t");
        const clean = url.origin + url.pathname + (url.search ? url.search : "");
        window.history.replaceState({}, document.title, clean);
      }
    } catch {}
  }, []);

  async function login(username: string, password: string) {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<{ access_token: string }>("/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role: "student" }),
      });
      setJwt(data.access_token);
      snack.setMsg("Logged in.");
      setScreen("select");
    } catch (err: any) {
      snack.setErr(err.message);
    } finally { setBusy(false); }
  }

  function signOut() {
    setJwt("");
    setLoadedExam(null);
    setLoadedOffering(undefined);
    setScreen("login");
    snack.setMsg("Signed out.");
  }

  async function loadExamById(examId: string, offering?: Offering) {
    snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Exam>(`/exams/${encodeURIComponent(examId)}`, {
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setLoadedExam(data);
      setLoadedOffering(offering); // NEW
      snack.setMsg("Exam loaded.");
      setScreen("exam");
    } catch (err: any) {
      snack.setErr(err.message);
    }
  }

  const theme = createTheme({
    palette: { mode: "light", primary: { main: "#3f51b5" } },
    shape: { borderRadius: 12 },
    components: {
      MuiPaper: { styleOverrides: { root: { borderRadius: 16 } } },
      MuiButton: { defaultProps: { disableRipple: true } },
    },
  });

  return (
    <ThemeProvider theme={theme}>
      {screen === "login" && <LoginScreen busy={busy} onLogin={login} />}
      {screen === "select" && jwt && <SelectScreen jwt={jwt} onBack={signOut} onLoadExam={loadExamById} />}
      {screen === "exam" && jwt && loadedExam && (
        <ExamScreen jwt={jwt} exam={loadedExam} offering={loadedOffering} onExit={() => setScreen("select")} />
      )}
    </ThemeProvider>
  );
}
