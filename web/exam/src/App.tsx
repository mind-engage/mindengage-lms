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
};

export type ExamSummary = {
  id: string;
  title: string;
  time_limit_sec?: number;
  created_at?: number;
  profile?: string;
};

/* -------------------- Helpers -------------------- */
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

function normType(t?: string): Question["type"] {
  return (t || "").replace(/-/g, "_") as Question["type"];
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
};

const QuestionCard = React.memo(function QuestionCard({ q, idx, value, onChange }: QuestionCardProps) {
  const qtype = normType(q.type);

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
          <RadioGroup value={value ?? ""} onChange={(e) => onChange(q.id, e.target.value)}>
            {q.choices?.map((c) => (
              <FormControlLabel
                key={c.id}
                value={c.id}
                control={<Radio />}
                label={<span dangerouslySetInnerHTML={htmlify(c.label_html)} />}
              />
            ))}
          </RadioGroup>
        )}

        {qtype === "true_false" && (
          <RadioGroup value={value ?? ""} onChange={(e) => onChange(q.id, e.target.value)}>
            {(["true", "false"] as const).map((v) => (
              <FormControlLabel key={v} value={v} control={<Radio />} label={<span style={{ textTransform: "capitalize" }}>{v}</span>} />
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
                      checked={checked}
                      onChange={(e) => {
                        const next = new Set(arr);
                        if (e.target.checked) next.add(c.id);
                        else next.delete(c.id);
                        onChange(q.id, Array.from(next));
                      }}
                    />
                  }
                  label={<span dangerouslySetInnerHTML={htmlify(c.label_html)} />}
                />
              );
            })}
          </FormGroup>
        )}

        {qtype === "short_word" && (
          <TextField
            fullWidth
            autoFocus
            placeholder="Your answer"
            value={value ?? ""}
            onChange={(e) => onChange(q.id, e.target.value)}
          />
        )}

        {qtype === "numeric" && (
          <TextField
            type="number"
            fullWidth
            placeholder="0"
            value={value ?? ""}
            onChange={(e) => onChange(q.id, e.target.value)}
          />
        )}

        {qtype === "essay" && (
          <TextField
            fullWidth
            multiline
            minRows={6}
            placeholder="Write your answer..."
            value={value ?? ""}
            onChange={(e) => onChange(q.id, e.target.value)}
          />
        )}
      </Stack>
    </Paper>
  );
}, (prev, next) => prev.q === next.q && prev.idx === next.idx && prev.value === next.value);
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
/* -------------------- Screen 2: Select -------------------- */
function SelectScreen({
  jwt,
  onBack,
  onLoadExam,
}: {
  jwt: string;
  onBack: () => void;
  onLoadExam: (id: string) => void;
}) {
  const [examId, setExamId] = useState("");
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [list, setList] = useState<ExamSummary[]>([]);
  const snack = useSnack();

  const fetchExams = useCallback(async (query: string) => {
    setBusy(true); snack.setErr(null); snack.setMsg(null);
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
      setBusy(false);
    }
  }, [jwt]);
  

  useEffect(() => {
    fetchExams("");
  }, [fetchExams]);

  function startFromRow(id: string) {
    setExamId(id);
    onLoadExam(id);
  }

  return (
    <Shell authed={true} title="Select exam or assignment" onSignOut={onBack}>
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
        {/* Manual ID entry */}
        <Box sx={{ width: { xs: '100%', md: `${(5 / 12) * 100}%` } }}>
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="h6" fontWeight={600}>Enter Exam ID</Typography>
            <Stack direction={{ xs: "column", sm: "row" }} spacing={2} alignItems={{ sm: "flex-end" }} sx={{ mt: 2 }}>
              <TextField label="Exam ID" value={examId} onChange={(e) => setExamId(e.target.value)} fullWidth />
              <Button variant="contained" onClick={() => onLoadExam(examId)} disableElevation disabled={!examId.trim()}>
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
              <Button variant="outlined" onClick={() => fetchExams(q)} disabled={busy}>
                {busy ? "Searching…" : "Search"}
              </Button>
              <Button variant="text" onClick={() => { setQ(""); fetchExams(""); }} disabled={busy}>
                Reset
              </Button>
            </Stack>

            <Stack spacing={1.25} sx={{ mt: 2, maxHeight: 420, overflowY: 'auto' }}>
              {list.length === 0 && !busy && (
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
                    <Button variant="outlined" onClick={() => startFromRow(e.id)}>Open</Button>
                  </Stack>
                </Paper>
              ))}
            </Stack>
          </Paper>
        </Box>
      </Stack>
      {snack.node}
    </Shell>
  );
}


/* -------------------- Screen 3: Exam -------------------- */
function ExamScreen({ jwt, exam, onExit }: { jwt: string; exam: Exam; onExit: () => void; }) {
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

  // Stable updater so memoized children don't re-render unnecessarily
  const updateResponse = useCallback((qid: string, val: any) => {
    setResponses((r) => {
      if (r[qid] === val) return r;
      return { ...r, [qid]: val };
    });
  }, []);

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

  // autosave (debounced, no per-keystroke stringify)
  useEffect(() => {
    if (!attempt) return;
    if (autosaveTRef.current) window.clearTimeout(autosaveTRef.current);
    autosaveTRef.current = window.setTimeout(() => { saveResponses(false); }, 1000) as unknown as number;
    return () => { if (autosaveTRef.current) window.clearTimeout(autosaveTRef.current); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [responses, attempt?.id]);

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

  return (
    <Shell authed={true} onSignOut={onExit} attempt={attempt} progressPct={progressPct} timer={formatTime(secondsLeft)} title={exam.title}>
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3}>
        {/* Left rail */}
        <Box sx={{ width: { xs: '100%', lg: '25%' }, display: { xs: "none", lg: "block" } }}>
          <Stack spacing={2} sx={{ position: "sticky", top: 88 }}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Stack direction="row" alignItems="center" justifyContent="space-between">
                <Typography variant="body2">
                  Time Limit: {Math.round((exam.time_limit_sec || 0) / 60)} min
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

              {attempt && exam.time_limit_sec ? (
                <LinearProgress
                  sx={{ mt: 1 }}
                  variant="determinate"
                  value={
                    Math.min(
                      100,
                      Math.max(
                        0,
                        ((exam.time_limit_sec - (secondsLeft || 0)) / exam.time_limit_sec) * 100
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
                  // give the list its own scroll
                  maxHeight: 'calc(100vh - 88px - 24px - 120px)', // viewport minus appbar & margins (tweak as needed)
                  overflowY: 'auto',
                  overscrollBehavior: 'contain',  // prevent wheel from scrolling the page
                }}
              >
                <Typography fontWeight={600} gutterBottom>Questions</Typography>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', mx: -0.5 }}>
                  {exam.questions.map((q, idx) => {
                    const r = responses[q.id];
                    const done = r != null && r !== "" && (!Array.isArray(r) || r.length > 0);
                    return (
                      <Box key={q.id} sx={{ width: '25%', p: 0.5 }}>
                        <Button
                          fullWidth
                          size="small"
                          variant={currentQ === idx ? "contained" : (done ? "outlined" : "text")}
                          onClick={() => setCurrentQ(idx)}
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

                <QuestionCard
                  key={exam.questions[currentQ].id}
                  q={exam.questions[currentQ]}
                  idx={currentQ}
                  value={responses[exam.questions[currentQ].id]}
                  onChange={updateResponse}
                />

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

/* -------------------- Root App -------------------- */
export default function StudentApp() {
  type Screen = "login" | "select" | "exam";
  const [screen, setScreen] = useState<Screen>("login");
  const [jwt, setJwt] = useState("");
  const [busy, setBusy] = useState(false);
  const [loadedExam, setLoadedExam] = useState<Exam | null>(null);
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
      {screen === "exam" && jwt && loadedExam && <ExamScreen jwt={jwt} exam={loadedExam} onExit={() => setScreen("select")} />}
    </ThemeProvider>
  );
}
