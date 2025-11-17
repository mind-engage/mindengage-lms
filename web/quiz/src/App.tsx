import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AppBar, Toolbar, Typography, Container, Box, Paper, TextField, Button, Stack,
  LinearProgress, Chip, Snackbar, Alert, Divider, Tooltip, RadioGroup, FormControlLabel,
  Radio, FormGroup, Checkbox, Dialog, DialogTitle, DialogContent, DialogContentText,
  DialogActions, Table, TableHead, TableRow, TableCell, TableBody
} from "@mui/material";
import { createTheme, ThemeProvider } from "@mui/material/styles";

/* ---------------- URL params ---------------- */
const url = new URL(window.location.href);
const API_BASE = process.env.REACT_APP_API_BASE || "http://localhost:8080/api";
const OFFERING_ID = url.searchParams.get("offering") || url.searchParams.get("o") || "";
const LINK_TOKEN  = url.searchParams.get("access_token") || url.searchParams.get("t") || "";
const SHOW_ANS_DEFAULT = url.searchParams.get("show_answers") === "1";

/* ---------------- Types ---------------- */
type Choice = { id: string; label_html?: string };
type Question = {
  id: string;
  type: "mcq_single" | "mcq_multi" | "true_false" | "short_word" | "numeric" | "essay";
  prompt_html?: string;
  prompt?: string;
  choices?: Choice[];
  points?: number;
};
type Exam = {
  id: string;
  title: string;
  time_limit_sec?: number;
  questions: Question[];
  policy?: any;
};
type Offering = {
  id: string;
  exam_id: string;
  course_id?: string;
  start_at?: string | null; // RFC3339
  end_at?: string | null;   // RFC3339
  time_limit_sec?: number | null;
  max_attempts: number;
  visibility: "course" | "public" | "link";
  state?: "not_started" | "active" | "ended";
  exam?: Exam; // included by /resolve
};
type GradeItem = {
  question_id: string;
  points: number;
  points_max: number;
  needs_manual: boolean;
  correct: boolean;
  feedback?: string[];
  correct_answer?: string[] | string;
};
type GradeResp = {
  score: number;
  score_max: number;
  items: GradeItem[];
};

/* ---------------- helpers ---------------- */
async function fetchJSON<T = any>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const res = await fetch(String(input), init);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}: ${await res.text()}`);
  if (res.status === 204) return undefined as T;
  return res.json();
}
function htmlify(s?: string) { return { __html: s || "" }; }
function normType(t?: string) { return (t || "").replace(/-/g, "_") as Question["type"]; }
function formatIso(s?: string | null) { return s ? new Date(s).toLocaleString() : ""; }
function fmtAnswer(ans: any) {
  if (ans == null) return "";
  if (Array.isArray(ans)) return ans.join(", ");
  return String(ans);
}
function formatTime(s?: number | null) {
  if (s == null) return null;
  const m = Math.floor(s / 60), r = s % 60;
  return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
}
function gradePct(g?: GradeResp | null) {
  if (!g || !g.score_max) return 0;
  return Math.round((g.score / g.score_max) * 100);
}

/* ---------------- snack ---------------- */
function useSnack() {
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  return {
    msg, err, setMsg, setErr,
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

/* ---------------- Question Card ---------------- */
function QuestionCard({
  q, idx, value, onChange, disabled,
}: {
  q: Question; idx: number; value: any; onChange: (qid: string, val: any) => void; disabled?: boolean;
}) {
  const t = normType(q.type);
  return (
    <Paper variant="outlined" sx={{ p: 2.5, mb: 2 }}>
      <Stack spacing={2}>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography variant="caption" color="text.secondary">
            Q{idx + 1} • {t.replace(/_/g, "-")}{q.points ? `  (${q.points} pt)` : ""}
          </Typography>
        </Stack>

        <Box sx={{ "& p": { my: 0.5 } }} dangerouslySetInnerHTML={htmlify(q.prompt_html || q.prompt)} />

        {t === "mcq_single" && (
          <RadioGroup value={value ?? ""} onChange={(e) => onChange(q.id, e.target.value)}>
            {(q.choices || []).map((c) => (
              <FormControlLabel
                key={c.id}
                value={c.id}
                control={<Radio disabled={disabled} />}
                label={<span dangerouslySetInnerHTML={htmlify(c.label_html)} />}
                disabled={disabled}
              />
            ))}
          </RadioGroup>
        )}

        {t === "true_false" && (
          <RadioGroup value={value ?? ""} onChange={(e) => onChange(q.id, e.target.value)}>
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

        {t === "mcq_multi" && (
          <FormGroup>
            {(q.choices || []).map((c) => {
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
                        if (e.target.checked) next.add(c.id); else next.delete(c.id);
                        onChange(q.id, Array.from(next));
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

        {t === "short_word" && (
          <TextField fullWidth placeholder="Your answer" value={value ?? ""} onChange={(e) => onChange(q.id, e.target.value)}
            disabled={disabled} InputProps={{ readOnly: disabled }} />
        )}

        {t === "numeric" && (
          <TextField type="number" fullWidth placeholder="0" value={value ?? ""} onChange={(e) => onChange(q.id, e.target.value)}
            disabled={disabled} InputProps={{ readOnly: disabled }} />
        )}

        {t === "essay" && (
          <TextField fullWidth multiline minRows={6} placeholder="Write your answer..." value={value ?? ""}
            onChange={(e) => onChange(q.id, e.target.value)} disabled={disabled} InputProps={{ readOnly: disabled }} />
        )}
      </Stack>
    </Paper>
  );
}

/* ---------------- Shell ---------------- */
function Shell({
  children, title, progressPct, timer,
}: { children: React.ReactNode; title?: string; progressPct?: number; timer?: string | null }) {
  return (
    <>
      <AppBar position="sticky" color="inherit" elevation={1} sx={{ backdropFilter: "saturate(180%) blur(6px)", background: "rgba(255,255,255,0.9)" }}>
        <Toolbar>
          <Box sx={{ width: 10, height: 10, bgcolor: "primary.main", borderRadius: 2, mr: 1.5 }} />
          <Typography variant="h6" sx={{ fontWeight: 600 }}>Ephemeral Quiz</Typography>
          {title && (
            <Typography variant="body2" color="text.secondary" sx={{ ml: 2, display: { xs: "none", md: "block" } }}>{title}</Typography>
          )}
          <Box sx={{ flexGrow: 1 }} />
          {typeof progressPct === "number" && (
            <Stack direction="row" alignItems="center" spacing={1} sx={{ mr: 2, display: { xs: "none", sm: "flex" } }}>
              <Typography variant="body2" color="text.secondary">Progress</Typography>
              <Box sx={{ width: 160 }}>
                <LinearProgress variant="determinate" value={Math.round(progressPct)} />
              </Box>
              <Chip size="small" label={`${Math.round(progressPct)}%`} />
            </Stack>
          )}
          {timer && <Chip label={`⏱ ${timer}`} variant="outlined" />}
        </Toolbar>
      </AppBar>
      <Container maxWidth="md" sx={{ py: 4 }}>{children}</Container>
    </>
  );
}

/* ---------------- Entry Screen (when no params) ---------------- */
function EntryScreen() {
  const [o, setO] = useState("");
  const [t, setT] = useState("");
  const go = () => {
    const next = new URL(window.location.href);
    next.searchParams.set("offering", o.trim());
    next.searchParams.set("access_token", t.trim());
    window.location.href = next.toString();
  };
  return (
    <Shell>
      <Paper elevation={1} sx={{ p: 3 }}>
        <Typography variant="h5" fontWeight={600}>Open link-offering</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Provide the Offering ID and Link Token. Alternatively, append them to the URL as
          <code> ?offering=…&access_token=…</code>.
        </Typography>
        <Stack direction={{ xs: "column", sm: "row" }} spacing={2} sx={{ mt: 2 }}>
          <TextField fullWidth label="Offering ID" value={o} onChange={(e) => setO(e.target.value)} />
          <TextField fullWidth label="Access token" value={t} onChange={(e) => setT(e.target.value)} />
        </Stack>
        <Stack direction="row" spacing={1.5} sx={{ mt: 2 }}>
          <Button variant="contained" onClick={go} disableElevation disabled={!o.trim() || !t.trim()}>Open</Button>
          <Button variant="text" onClick={() => { setO(""); setT(""); }}>Reset</Button>
        </Stack>
        <Divider sx={{ my: 2 }} />
        <Typography variant="caption" color="text.secondary">
          API base: <code>{API_BASE}</code>
        </Typography>
      </Paper>
    </Shell>
  );
}

/* ---------------- Results Table ---------------- */
function ResultsTable({ result, showAnswers }:{ result: GradeResp; showAnswers: boolean }) {
  return (
    <Paper elevation={1} sx={{ p: 2 }}>
      <Stack direction="row" alignItems="center" spacing={2} sx={{ mb: 1 }}>
        <Typography variant="h6" fontWeight={600}>Result</Typography>
        <Chip color="primary" label={`${gradePct(result)}%`} />
        <Typography variant="body2" color="text.secondary">
          {Math.round((result.score + Number.EPSILON) * 10) / 10} / {Math.round((result.score_max + Number.EPSILON) * 10) / 10}
        </Typography>
      </Stack>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Q#</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>Points</TableCell>
            <TableCell>Feedback</TableCell>
            {showAnswers && <TableCell>Answer</TableCell>}
          </TableRow>
        </TableHead>
        <TableBody>
          {result.items.map((it, i) => (
            <TableRow key={it.question_id}>
              <TableCell>Q{i + 1}</TableCell>
              <TableCell>
                {it.needs_manual
                  ? <Chip size="small" color="warning" label="Needs manual" />
                  : it.correct
                    ? <Chip size="small" color="success" label="Correct" />
                    : <Chip size="small" color="error" label="Incorrect" />}
              </TableCell>
              <TableCell>{it.points} / {it.points_max}</TableCell>
              <TableCell>{(it.feedback || []).join("; ")}</TableCell>
              {showAnswers && <TableCell><code>{fmtAnswer(it.correct_answer)}</code></TableCell>}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Paper>
  );
}

/* ---------------- Main App ---------------- */
export default function App() {
  const snack = useSnack();

  const [offering, setOffering] = useState<Offering | null>(null);
  const [exam, setExam]         = useState<Exam | null>(null);
  const [busy, setBusy]         = useState(false);
  const [err, setErr]           = useState<string | null>(null);

  const [responses, setResponses] = useState<Record<string, any>>({});
  const [result, setResult]       = useState<GradeResp | null>(null);
  const [showAnswers, setShowAnswers] = useState<boolean>(SHOW_ANS_DEFAULT);

  // client-side timer (display only; server still enforces open window)
  const [secondsLeft, setSecondsLeft] = useState<number | null>(null);
  const timerRef = useRef<number | null>(null);

  const total = exam?.questions.length || 0;
  const answered = useMemo(() => {
    if (!exam) return 0;
    return exam.questions.reduce((n, q) => {
      const v = responses[q.id];
      const has = !(v == null || v === "" || (Array.isArray(v) && v.length === 0));
      return n + (has ? 1 : 0);
    }, 0);
  }, [exam, responses]);
  const progressPct = total ? (answered / total) * 100 : 0;

  const disabledByState = offering?.state === "not_started" || offering?.state === "ended";

  const updateResponse = useCallback((qid: string, val: any) => {
    setResponses(prev => (prev[qid] === val ? prev : ({ ...prev, [qid]: val })));
  }, []);

  // Fetch offering+exam from /resolve
  useEffect(() => {
    if (!OFFERING_ID || !LINK_TOKEN) return;
    (async () => {
      setBusy(true); setErr(null); snack.setMsg(null);
      try {
        const o = await fetchJSON<Offering>(`${API_BASE}/offerings/${encodeURIComponent(OFFERING_ID)}/resolve?access_token=${encodeURIComponent(LINK_TOKEN)}`);
        if (!o || !o.exam) throw new Error("Resolve payload missing 'exam'.");
        setOffering(o);
        setExam(o.exam);
        // timer from offering override or exam default
        const tl = (typeof o.time_limit_sec === "number" && o.time_limit_sec > 0)
          ? o.time_limit_sec
          : (typeof o.exam.time_limit_sec === "number" && o.exam.time_limit_sec > 0 ? o.exam.time_limit_sec : null);
        setSecondsLeft(tl as any);
      } catch (e: any) {
        setErr(e?.message || String(e));
      } finally { setBusy(false); }
    })();
  }, []);

  // client timer tick
  useEffect(() => {
    if (secondsLeft == null) return;
    if (timerRef.current) window.clearInterval(timerRef.current);
    timerRef.current = window.setInterval(() => {
      setSecondsLeft(s => (s == null ? s : Math.max(0, s - 1)));
    }, 1000) as unknown as number;
    return () => { if (timerRef.current) window.clearInterval(timerRef.current); };
  }, [secondsLeft !== null]); // eslint-disable-line react-hooks/exhaustive-deps

  async function gradeNow(withAnswers = showAnswers) {
    if (!OFFERING_ID || !LINK_TOKEN) return;
    setBusy(true); setErr(null); setResult(null);
    try {
      const qs = new URLSearchParams({ access_token: LINK_TOKEN });
      if (withAnswers) qs.set("show_answers", "1");
      const out = await fetchJSON<GradeResp>(
        `${API_BASE}/offerings/${encodeURIComponent(OFFERING_ID)}/grade_ephemeral?${qs}`,
        { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ responses }) }
      );
      setResult(out || null);
      snack.setMsg("Graded.");
    } catch (e: any) {
      setErr(e?.message || String(e));
    } finally { setBusy(false); }
  }

  function resetAll() {
    setResponses({});
    setResult(null);
  }

  const theme = createTheme({
    palette: { mode: "light", primary: { main: "#3f51b5" } },
    shape: { borderRadius: 12 },
    components: {
      MuiPaper: { styleOverrides: { root: { borderRadius: 16 } } },
      MuiButton: { defaultProps: { disableRipple: true } },
    },
  });

  if (!OFFERING_ID || !LINK_TOKEN) {
    return (
      <ThemeProvider theme={theme}>
        <EntryScreen />
      </ThemeProvider>
    );
  }

  return (
    <ThemeProvider theme={theme}>
      <Shell title={exam?.title} progressPct={progressPct} timer={formatTime(secondsLeft)}>
        {err && <Alert severity="error" sx={{ mb: 2 }}>{err}</Alert>}
        {busy && <Alert severity="info" sx={{ mb: 2 }}>Working…</Alert>}

        {offering && (
          <Paper elevation={1} sx={{ p: 2, mb: 2 }}>
            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "center" }}>
              <Typography variant="body2">
                <strong>Offering:</strong> <code>{offering.id}</code> • <strong>Exam:</strong> <code>{offering.exam_id}</code>
                {offering.start_at && <> • Starts: {formatIso(offering.start_at)}</>}
                {offering.end_at && <> • Ends: {formatIso(offering.end_at)}</>}
                {typeof offering.time_limit_sec === "number" && <> • ⏱ {Math.round((offering.time_limit_sec || 0) / 60)} min</>}
                {offering.state && <> • State: {offering.state}</>}
              </Typography>
              <Box sx={{ flexGrow: 1 }} />
              {disabledByState && (
                <Chip color="warning" label={offering.state === "not_started" ? "Not started" : "Ended"} />
              )}
            </Stack>
          </Paper>
        )}

        {exam ? (
          <>
            <Paper elevation={1} sx={{ p: 3 }}>
              {exam.questions.map((q, i) => (
                <QuestionCard
                  key={q.id}
                  q={q}
                  idx={i}
                  value={responses[q.id]}
                  onChange={updateResponse}
                  disabled={false}
                />
              ))}

              <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mt: 2 }}>
                <Tooltip title={disabledByState ? "Offering is not active" : ""}>
                  <span>
                    <Button variant="contained" disableElevation onClick={() => gradeNow(false)} disabled={busy || disabledByState}>
                      Grade
                    </Button>
                  </span>
                </Tooltip>
                <Tooltip title={disabledByState ? "Offering is not active" : ""}>
                  <span>
                    <Button variant="outlined" onClick={() => gradeNow(true)} disabled={busy || disabledByState}>
                      Grade + Answers
                    </Button>
                  </span>
                </Tooltip>
                <Button variant="text" onClick={resetAll} disabled={busy}>Reset</Button>
                <Box sx={{ flexGrow: 1 }} />
                <Typography variant="caption" color="text.secondary">Answered {answered} of {total}</Typography>
              </Stack>
            </Paper>

            {result && (
              <Box sx={{ mt: 2 }}>
                <ResultsTable result={result} showAnswers={showAnswers} />
              </Box>
            )}
          </>
        ) : (
          <Paper elevation={1} sx={{ p: 3 }}>
            <Typography variant="body2" color="text.secondary">Waiting for exam content…</Typography>
            <Typography variant="caption" color="text.secondary">
              Ensure <code>/api/offerings/{'{id}'}/resolve</code> returns <code>{"{ exam: {...} }"}</code>.
            </Typography>
          </Paper>
        )}

        {/* tiny info dialog */}
        <InfoDialog />

        {snack.node}
      </Shell>
    </ThemeProvider>
  );
}

/* ---------------- small info dialog ---------------- */
function InfoDialog() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Box sx={{ textAlign: "center", mt: 3 }}>
        <Button size="small" onClick={() => setOpen(true)}>About</Button>
      </Box>
      <Dialog open={open} onClose={() => setOpen(false)}>
        <DialogTitle>Ephemeral Quiz</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This app uses two endpoints:
            <br />
            <code>GET /offerings/&lt;id&gt;/resolve?access_token=…</code> and{" "}
            <code>POST /offerings/&lt;id&gt;/grade_ephemeral?access_token=…</code>.
            <br />
            Add <code>?offering=…&amp;access_token=…</code> to the URL to launch.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)} autoFocus>Close</Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
