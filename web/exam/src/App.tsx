import React, { useEffect, useMemo, useRef, useState } from "react";
// REMOVED: No more Grid import is necessary.
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
  Divider,
  IconButton,
  Snackbar,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  ToggleButtonGroup,
  ToggleButton,
  FormGroup,
  FormControlLabel,
  Checkbox,
  Radio,
  RadioGroup,
} from "@mui/material";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import LogoutIcon from "@mui/icons-material/Logout";


const API_BASE = (import.meta as any).env?.VITE_API_BASE || "http://localhost:8080";

// -------------------- Types --------------------
export type Exam = {
  id: string;
  title: string;
  time_limit_sec?: number;
  questions: Question[];
};

export type Question = {
  id: string;
  type: "mcq_single" | "mcq_multi" | "true_false" | "short_word" | "numeric" | "essay";
  prompt_html?: string;
  prompt?: string; // legacy
  choices?: { id: string; label_html?: string }[];
  points?: number;
};

export type Attempt = {
  id: string;
  exam_id: string;
  user_id: string;
  status: "in_progress" | "submitted" | string;
  score?: number;
};

// -------------------- Helpers --------------------
async function api<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, opts);
  if (!res.ok) {
    const t = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${t}`);
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

// -------------------- Shared Shell --------------------
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
          {attempt && timer && (
            <Chip label={`⏱ ${timer}`} variant="outlined" sx={{ mr: 2 }} />
          )}
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

// -------------------- Screen 1: Login --------------------
function LoginScreen({ busy, onLogin }: { busy: boolean; onLogin: (u: string, p: string, r: "student" | "teacher" | "admin") => void; }) {
  const [username, setUsername] = useState("student");
  const [password, setPassword] = useState("student");
  const [role, setRole] = useState<"student" | "teacher" | "admin">("student");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onLogin(username, password, role);
  }

  return (
    <Shell authed={false} title="Sign in">
      {/* REPLACED: Grid with Stack and Box */}
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
        <Box sx={{ width: { xs: '100%', md: `${(7 / 12) * 100}%`, lg: '50%' } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="h5" fontWeight={600}>Sign in</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>Use your test credentials to continue.</Typography>
            <Box component="form" onSubmit={submit} sx={{ mt: 2 }}>
              <Stack spacing={2}>
                <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} fullWidth />
                <TextField label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} fullWidth />
                <FormControl fullWidth>
                  <InputLabel id="role-lbl">Role</InputLabel>
                  <Select labelId="role-lbl" label="Role" value={role} onChange={(e) => setRole(e.target.value as any)}>
                    <MenuItem value="student">student</MenuItem>
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

// -------------------- Screen 2: Select --------------------
function SelectScreen({ onBack, onLoadExam }: { onBack: () => void; onLoadExam: (id: string) => void; }) {
  const [examId, setExamId] = useState("exam-101");

  return (
    <Shell authed={true} title="Select exam or assignment" onSignOut={onBack}>
      {/* REPLACED: Grid with Stack and Box */}
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
        <Box sx={{ width: { xs: '100%', md: `${(8 / 12) * 100}%` } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="h6" fontWeight={600}>Enter Exam ID</Typography>
            <Stack direction={{ xs: "column", sm: "row" }} spacing={2} alignItems={{ sm: "flex-end" }} sx={{ mt: 2 }}>
              <TextField label="Exam ID" value={examId} onChange={(e) => setExamId(e.target.value)} fullWidth />
              <Button variant="contained" onClick={() => onLoadExam(examId)} disableElevation>Load</Button>
            </Stack>
            <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>Example: <Box component="code" sx={{ px: 0.5, py: 0.25, bgcolor: "action.hover", borderRadius: 1 }}>exam-101</Box></Typography>
          </Paper>
        </Box>
        <Box sx={{ width: { xs: '100%', md: `${(4 / 12) * 100}%` } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography fontWeight={600}>Recent (example)</Typography>
            <Stack spacing={1.2} sx={{ mt: 1.5 }}>
              {["exam-101", "exam-physics", "assignment-essay"].map((id) => (
                <Button key={id} variant="outlined" onClick={() => onLoadExam(id)}>{id}</Button>
              ))}
            </Stack>
          </Paper>
        </Box>
      </Stack>
    </Shell>
  );
}

// -------------------- Screen 3: Exam --------------------
function ExamScreen({ jwt, exam, onExit }: { jwt: string; exam: Exam; onExit: () => void; }) {
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const [responses, setResponses] = useState<Record<string, any>>({});
  const [busy, setBusy] = useState(false);
  const [currentQ, setCurrentQ] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [secondsLeft, setSecondsLeft] = useState<number | null>(null);
  const timerRef = useRef<number | null>(null);

  const total = exam?.questions.length ?? 0;
  const answered = useMemo(() => {
    if (!exam) return 0;
    return exam.questions.reduce((n, q) => (responses[q.id] == null || responses[q.id] === "" || (Array.isArray(responses[q.id]) && responses[q.id].length === 0) ? n : n + 1), 0);
  }, [exam, responses]);
  const progressPct = total ? (answered / total) * 100 : 0;

  const snack = useSnack();

  // actions
  async function startAttempt() {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Attempt>("/attempts", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify({ exam_id: exam.id, user_id: "student" }),
      });
      setAttempt(data);
      snack.setMsg(`Attempt ${data.id} started.`);
      if (exam.time_limit_sec) setSecondsLeft(exam.time_limit_sec);
    } catch (err: any) {
      snack.setErr(err.message);
    } finally { setBusy(false); }
  }

  async function saveResponses(manual = false) {
    if (!attempt) return;
    try {
      await api(`/attempts/${attempt.id}/responses`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${jwt}` },
        body: JSON.stringify(responses),
      });
      if (manual) snack.setMsg("Responses saved.");
    } catch (err: any) {
      snack.setErr(err.message);
    }
  }

  async function submitAttempt() {
    if (!attempt) return;
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Attempt>(`/attempts/${attempt.id}/submit`, {
        method: "POST",
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setAttempt(data);
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

  // autosave
  const respJsonRef = useRef("");
  useEffect(() => {
    if (!attempt) return;
    const s = JSON.stringify(responses);
    if (s === respJsonRef.current) return;
    respJsonRef.current = s;
    const t = setTimeout(() => saveResponses(false), 1200);
    return () => clearTimeout(t);
  }, [responses, attempt]);

  // timer
  useEffect(() => {
    if (secondsLeft == null) return;
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
  }, [attempt?.id]);

  function setResp(qid: string, val: any) {
    setResponses((r) => ({ ...r, [qid]: val }));
  }

  function QuestionCard({ q, idx }: { q: Question; idx: number }) {
    const current = responses[q.id];
    return (
      <Paper variant="outlined" sx={{ p: 2.5 }}>
        <Stack spacing={2}>
          <Stack direction="row" justifyContent="space-between" alignItems="center">
            <Typography variant="caption" color="text.secondary">
              Q{idx + 1} • {q.type.replace("_", "-")}{q.points ? `  ( ${q.points} pt )` : ""}
            </Typography>
          </Stack>
          <Box sx={{ "& p": { my: 0.5 } }} dangerouslySetInnerHTML={htmlify(q.prompt_html || q.prompt)} />

          {q.type === "mcq_single" && (
            <RadioGroup value={current ?? ""} onChange={(e) => setResp(q.id, e.target.value)}>
              {q.choices?.map((c) => (
                <FormControlLabel key={c.id} value={c.id} control={<Radio />} label={<span dangerouslySetInnerHTML={htmlify(c.label_html)} />} />
              ))}
            </RadioGroup>
          )}

          {q.type === "true_false" && (
            <RadioGroup value={current ?? ""} onChange={(e) => setResp(q.id, e.target.value)}>
              {(["true", "false"] as const).map((v) => (
                <FormControlLabel key={v} value={v} control={<Radio />} label={<span style={{ textTransform: "capitalize" }}>{v}</span>} />
              ))}
            </RadioGroup>
          )}

          {q.type === "mcq_multi" && (
            <FormGroup>
              {q.choices?.map((c) => {
                const arr: string[] = Array.isArray(current) ? current : [];
                const checked = arr.includes(c.id);
                return (
                  <FormControlLabel
                    key={c.id}
                    control={<Checkbox checked={checked} onChange={(e) => {
                      const next = new Set(arr);
                      if (e.target.checked) next.add(c.id); else next.delete(c.id);
                      setResp(q.id, Array.from(next));
                    }} />}
                    label={<span dangerouslySetInnerHTML={htmlify(c.label_html)} />}
                  />
                );
              })}
            </FormGroup>
          )}

          {q.type === "short_word" && (
            <TextField fullWidth placeholder="Your answer" value={current || ""} onChange={(e) => setResp(q.id, e.target.value)} />
          )}

          {q.type === "numeric" && (
            <TextField type="number" fullWidth placeholder="0" value={current ?? ""} onChange={(e) => setResp(q.id, e.target.value)} />
          )}

          {q.type === "essay" && (
            <TextField fullWidth multiline minRows={6} placeholder="Write your answer..." value={current || ""} onChange={(e) => setResp(q.id, e.target.value)} />
          )}
        </Stack>
      </Paper>
    );
  }

  return (
    <Shell authed={true} onSignOut={onExit} attempt={attempt} progressPct={progressPct} timer={formatTime(secondsLeft)} title={exam.title}>
      {/* REPLACED: Grid with Stack and Box */}
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3}>
        {/* Left rail */}
        <Box sx={{ width: { xs: '100%', lg: '25%' }, display: { xs: "none", lg: "block" } }}>
          <Stack spacing={2} sx={{ position: "sticky", top: 88 }}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Typography variant="caption" color="text.secondary">Step</Typography>
              <Box component="ol" sx={{ mt: 1, pl: 2 }}>
                <li>Sign in</li>
                <li>Select exam</li>
                <li><b>Take & submit</b></li>
              </Box>
            </Paper>

            {exam && attempt && (
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography fontWeight={600} gutterBottom>Questions</Typography>
                {/* REPLACED: Nested Grid with a flex-wrapping Box */}
                <Box sx={{ display: 'flex', flexWrap: 'wrap', mx: -0.5 }}>
                  {exam.questions.map((q, idx) => {
                    const r = responses[q.id];
                    const done = r != null && r !== "" && (!Array.isArray(r) || r.length > 0);
                    return (
                      <Box key={q.id} sx={{ width: '25%', p: 0.5 }}>
                        <Button fullWidth size="small" variant={currentQ === idx ? "contained" : (done ? "outlined" : "text")} onClick={() => setCurrentQ(idx)}>
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
                {exam.time_limit_sec ? (
                  <Typography variant="caption" color="text.secondary">Time limit: {Math.round((exam.time_limit_sec || 0) / 60)} min</Typography>
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
                    <Select labelId="jump-q" label="Jump to question" value={currentQ} onChange={(e) => setCurrentQ(Number(e.target.value))}>
                      {exam.questions.map((q, idx) => (
                        <MenuItem key={q.id} value={idx}>Q{idx + 1}</MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Box>

                <QuestionCard q={exam.questions[currentQ]} idx={currentQ} />

                <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mt: 2 }}>
                  <Button variant="outlined" onClick={() => setCurrentQ((i) => Math.max(0, i - 1))} disabled={currentQ === 0}>← Prev</Button>
                  <Button variant="outlined" onClick={() => setCurrentQ((i) => Math.min((exam.questions.length - 1), i + 1))} disabled={currentQ >= exam.questions.length - 1}>Next →</Button>
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
                <Button onClick={() => saveResponses(true)} variant="outlined">Save now</Button>
                <Button component="label" variant="outlined" disabled={uploading}>
                  {uploading ? "Uploading…" : "Upload scan"}
                  <input type="file" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) uploadAsset(f); }} />
                </Button>
                <Box sx={{ flexGrow: 1 }} />
                <Button onClick={() => setConfirmOpen(true)} variant="contained" disableElevation>Submit</Button>
                {attempt?.score !== undefined && (
                  <Chip color="success" label={`Score: ${attempt.score}`} sx={{ ml: 1 }} />
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

      {snack.node}
    </Shell>
  );
}

// -------------------- Root App --------------------
export default function StudentApp() {
  type Screen = "login" | "select" | "exam";
  const [screen, setScreen] = useState<Screen>("login");
  const [jwt, setJwt] = useState("");
  const [busy, setBusy] = useState(false);
  const [loadedExam, setLoadedExam] = useState<Exam | null>(null);
  const snack = useSnack();

  async function login(username: string, password: string, role: "student" | "teacher" | "admin") {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<{ access_token: string }>("/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role }),
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
    setScreen("login");
    snack.setMsg("Signed out.");
  }

  async function loadExamById(examId: string) {
    snack.setErr(null); snack.setMsg(null);
    try {
      const data = await api<Exam>(`/exams/${encodeURIComponent(examId)}`, {
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setLoadedExam(data);
      snack.setMsg("Exam loaded.");
      setScreen("exam");
    } catch (err: any) {
      snack.setErr(err.message);
    }
  }

  const theme = createTheme({
    palette: {
      mode: "light",
      primary: { main: "#3f51b5" },
    },
    shape: { borderRadius: 12 },
    components: {
      MuiPaper: { styleOverrides: { root: { borderRadius: 16 } } },
      MuiButton: { defaultProps: { disableRipple: true } },
    },
  });

  return (
    <ThemeProvider theme={theme}>
      {screen === "login" && <LoginScreen busy={busy} onLogin={login} />}
      {screen === "select" && jwt && (
        <SelectScreen onBack={signOut} onLoadExam={loadExamById} />
      )}
      {screen === "exam" && jwt && loadedExam && (
        <ExamScreen jwt={jwt} exam={loadedExam} onExit={() => setScreen("select")} />
      )}
    </ThemeProvider>
  );
}