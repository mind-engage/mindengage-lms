import React, { useEffect, useMemo, useRef, useState } from "react";

// === MindEngage • Student (UX Refresh) ===
// One-file component with improved flow, layout, and styling.
// - Clear 3-step journey: Sign in → Load exam → Take & submit
// - Sticky action bar, question navigator, progress, timer, autosave
// - Better empty states & error handling
// - TailwindCSS for all styling (no extra imports)

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

function classNames(...arr: Array<string | false | null | undefined>) {
  return arr.filter(Boolean).join(" ");
}

// -------------------- Main --------------------
export default function StudentExamApp() {
  // auth
  const [jwt, setJwt] = useState("");
  const [role, setRole] = useState<"student" | "teacher" | "admin">("student");
  const [username, setUsername] = useState("student");
  const [password, setPassword] = useState("student");

  // exam state
  const [examId, setExamId] = useState("exam-101");
  const [exam, setExam] = useState<Exam | null>(null);
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const [responses, setResponses] = useState<Record<string, any>>({});

  // ui state
  const [busy, setBusy] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [currentQ, setCurrentQ] = useState(0);
  const [uploading, setUploading] = useState(false);

  // timer
  const [secondsLeft, setSecondsLeft] = useState<number | null>(null);
  const timerRef = useRef<number | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);

  const authed = useMemo(() => Boolean(jwt), [jwt]);

  // -------------- Actions --------------
  async function login(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null); setToast(null);
    try {
      const data = await api<{ access_token: string }>("/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role }),
      });
      setJwt(data.access_token);
      setToast("Logged in.");
    } catch (err: any) {
      setError(err.message);
    } finally { setBusy(false); }
  }

  async function fetchExam() {
    if (!examId) return;
    setBusy(true); setError(null); setToast(null);
    try {
      const data = await api<Exam>(`/exams/${encodeURIComponent(examId)}`, {
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setExam(data);
      setResponses({});
      setAttempt(null);
      setCurrentQ(0);
      setToast("Exam loaded.");
    } catch (err: any) {
      setError(err.message);
    } finally { setBusy(false); }
  }

  async function startAttempt() {
    if (!exam) return;
    setBusy(true); setError(null); setToast(null);
    try {
      const data = await api<Attempt>("/attempts", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${jwt}`,
        },
        body: JSON.stringify({ exam_id: exam.id, user_id: username || "student" }),
      });
      setAttempt(data);
      setToast(`Attempt ${data.id} started.`);
      // local timer starts when attempt starts (best-effort)
      if (exam.time_limit_sec) setSecondsLeft(exam.time_limit_sec);
    } catch (err: any) {
      setError(err.message);
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
      if (manual) setToast("Responses saved.");
    } catch (err: any) {
      setError(err.message);
    }
  }

  async function submitAttempt() {
    if (!attempt) return;
    //if (!confirm("Submit now? You won’t be able to edit after submitting.")) return;
    setBusy(true); setError(null); setToast(null);
    try {
      const data = await api<Attempt>(`/attempts/${attempt.id}/submit`, {
        method: "POST",
        headers: { Authorization: `Bearer ${jwt}` },
      });
      setAttempt(data);
      setToast(`Submitted. Score: ${data.score ?? 0}`);
    } catch (err: any) {
      setError(err.message);
    } finally { setBusy(false); }
  }

  async function uploadAsset(file: File) {
    if (!attempt) { setError("Start an attempt first."); return; }
    setUploading(true); setError(null); setToast(null);
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
      setToast(`Uploaded: ${data.key}`);
    } catch (err: any) {
      setError(err.message);
    } finally { setUploading(false); }
  }

  function logout() {
    setJwt("");
    setExam(null);
    setAttempt(null);
    setResponses({});
    setSecondsLeft(null);
    setToast("Signed out.");
  }


  function handleSubmitClick() {
    setShowConfirm(true);
  }

  async function handleConfirmSubmit() {
    async function handleConfirmSubmit() {
      setShowConfirm(false);
      await submitAttempt(); // use your existing submit function
    }
  }

  // -------------- Autosave (debounced) --------------
  const respJsonRef = useRef("");
  useEffect(() => {
    if (!attempt) return;
    const s = JSON.stringify(responses);
    if (s === respJsonRef.current) return;
    respJsonRef.current = s;
    const t = setTimeout(() => saveResponses(false), 1200); // 1.2s debounce
    return () => clearTimeout(t);
  }, [responses, attempt]);

  // -------------- Timer tick --------------
  useEffect(() => {
    if (secondsLeft == null) return;
    if (timerRef.current) window.clearInterval(timerRef.current);
    timerRef.current = window.setInterval(() => {
      setSecondsLeft((prev) => {
        if (prev == null) return prev;
        if (prev <= 1) {
          window.clearInterval(timerRef.current!);
          // auto-submit when time is up (best-effort)
          submitAttempt();
          return 0;
        }
        return prev - 1;
      });
    }, 1000) as unknown as number;
    return () => { if (timerRef.current) window.clearInterval(timerRef.current); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [attempt?.id]);

  // -------------- Render helpers --------------
  function setResp(qid: string, val: any) {
    setResponses((r) => ({ ...r, [qid]: val }));
  }

  function renderQuestion(q: Question) {
    const prompt = q.prompt_html || q.prompt || "";
    const current = responses[q.id];
    const index = exam?.questions.findIndex((x) => x.id === q.id) ?? 0;

    return (
      <section key={q.id} className="rounded-2xl p-5 shadow mb-5 bg-white border border-gray-100">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-xs text-gray-500 mb-1">Q{index + 1} • {q.type.replace("_", "-")}{q.points ? `  ( ${q.points} pt )` : ""}</div>
            <div className="prose max-w-none" dangerouslySetInnerHTML={htmlify(prompt)} />
          </div>
        </div>

        {/* Inputs by type */}
        {q.type === "mcq_single" && (
          <div className="mt-4 space-y-2">
            {q.choices?.map((c) => (
              <label key={c.id} className="flex items-center gap-3 cursor-pointer select-none">
                <input type="radio" name={q.id} checked={current === c.id} onChange={() => setResp(q.id, c.id)} />
                <span dangerouslySetInnerHTML={htmlify(c.label_html)} />
              </label>
            ))}
          </div>
        )}

        {q.type === "mcq_multi" && (
          <div className="mt-4 space-y-2">
            {q.choices?.map((c) => {
              const arr: string[] = Array.isArray(current) ? current : [];
              const checked = arr.includes(c.id);
              return (
                <label key={c.id} className="flex items-center gap-3 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={(e) => {
                      const next = new Set(arr);
                      if (e.target.checked) next.add(c.id); else next.delete(c.id);
                      setResp(q.id, Array.from(next));
                    }}
                  />
                  <span dangerouslySetInnerHTML={htmlify(c.label_html)} />
                </label>
              );
            })}
          </div>
        )}

        {q.type === "true_false" && (
          <div className="mt-4 flex gap-6">
            {(["true", "false"] as Array<"true" | "false">).map((v) => (
              <label key={v} className="flex items-center gap-3 cursor-pointer select-none">
                <input type="radio" name={q.id} checked={current === v} onChange={() => setResp(q.id, v)} />
                <span className="capitalize">{v}</span>
              </label>
            ))}
          </div>
        )}

        {q.type === "short_word" && (
          <input
            className="mt-4 w-full border rounded-xl px-3 py-2 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            placeholder="Your answer"
            value={current || ""}
            onChange={(e) => setResp(q.id, e.target.value)}
          />
        )}

        {q.type === "numeric" && (
          <input
            type="number"
            className="mt-4 w-full border rounded-xl px-3 py-2 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            placeholder="0"
            value={current ?? ""}
            onChange={(e) => setResp(q.id, e.target.value)}
          />
        )}

        {q.type === "essay" && (
          <textarea
            className="mt-4 w-full border rounded-xl px-3 py-2 min-h-[140px] focus:outline-none focus:ring-2 focus:ring-indigo-500"
            placeholder="Write your answer..."
            value={current || ""}
            onChange={(e) => setResp(q.id, e.target.value)}
          />
        )}
      </section>
    );
  }

  // -------------- Layout --------------
  const total = exam?.questions.length ?? 0;
  const answered = useMemo(() => {
    if (!exam) return 0;
    return exam.questions.reduce((n, q) => (responses[q.id] == null || responses[q.id] === "" || (Array.isArray(responses[q.id]) && responses[q.id].length === 0) ? n : n + 1), 0);
  }, [exam, responses]);

  function formatTime(s?: number | null) {
    if (s == null) return "--:--";
    const m = Math.floor(s / 60);
    const r = s % 60;
    return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
  }

  return (
    <div className="min-h-screen bg-gradient-to-b from-gray-50 to-gray-100 text-gray-900">
      {/* Top bar */}
      <header className="sticky top-0 z-40 backdrop-blur bg-white/70 border-b border-gray-200">
        <div className="max-w-6xl mx-auto px-4 py-3 flex items-center gap-3">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-xl bg-indigo-600" />
            <h1 className="font-semibold">MindEngage • Student</h1>
          </div>
          <div className="ml-auto flex items-center gap-3 text-sm">
            {attempt && (
              <div className="hidden sm:flex items-center gap-2 text-gray-600">
                <span className="font-medium">Progress</span>
                <div className="w-40 h-2 bg-gray-200 rounded-full overflow-hidden"><div className="h-full bg-indigo-600" style={{ width: `${Math.round((answered / (total || 1)) * 100)}%` }} /></div>
                <span>{answered}/{total}</span>
              </div>
            )}
            {attempt && exam?.time_limit_sec ? (
              <div className="px-3 py-1 rounded-lg border bg-white font-mono">⏱ {formatTime(secondsLeft)}</div>
            ) : null}
            {authed ? (
              <button onClick={logout} className="px-3 py-1 rounded-lg border hover:bg-gray-50">Sign out</button>
            ) : null}
          </div>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-4 py-6 grid grid-cols-1 lg:grid-cols-[260px,1fr] gap-6">
        {/* Left rail */}
        <aside className="hidden lg:block">
          <div className="sticky top-20 space-y-4">
            <div className="bg-white rounded-2xl shadow border p-4">
              <div className="text-xs text-gray-500">Step</div>
              <ol className="mt-2 text-sm space-y-2">
                <li className={classNames("flex items-center gap-2", authed ? "text-gray-700" : "font-semibold")}>1. Sign in</li>
                <li className={classNames("flex items-center gap-2", exam ? "text-gray-700" : "font-semibold")}>2. Load exam</li>
                <li className={classNames("flex items-center gap-2", attempt ? "text-gray-700" : "font-semibold")}>3. Take & submit</li>
              </ol>
            </div>

            {exam && attempt && (
              <div className="bg-white rounded-2xl shadow border p-4">
                <div className="text-sm font-medium mb-2">Questions</div>
                <div className="grid grid-cols-5 gap-2">
                  {exam.questions.map((q, idx) => {
                    const done = responses[q.id] != null && responses[q.id] !== "" && (!Array.isArray(responses[q.id]) || responses[q.id].length > 0);
                    return (
                      <button
                        key={q.id}
                        onClick={() => setCurrentQ(idx)}
                        className={classNames(
                          "aspect-square rounded-lg text-sm border flex items-center justify-center",
                          currentQ === idx ? "bg-indigo-600 text-white border-indigo-600" : done ? "bg-emerald-50 border-emerald-200 text-emerald-700" : "bg-white hover:bg-gray-50"
                        )}
                        title={`Go to Q${idx + 1}`}
                      >
                        {idx + 1}
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        </aside>

        {/* Right content */}
        <section>
          {!authed && (
            <form onSubmit={login} className="bg-white rounded-2xl shadow p-6 border max-w-lg">
              <h2 className="text-xl font-semibold">Sign in</h2>
              <p className="text-sm text-gray-600 mt-1">Use your test credentials to continue.</p>
              <div className="mt-4 grid gap-3">
                <div>
                  <label className="text-sm">Username</label>
                  <input className="mt-1 w-full border rounded-xl px-3 py-2" value={username} onChange={(e) => setUsername(e.target.value)} />
                </div>
                <div>
                  <label className="text-sm">Password</label>
                  <input type="password" className="mt-1 w-full border rounded-xl px-3 py-2" value={password} onChange={(e) => setPassword(e.target.value)} />
                </div>
                <div>
                  <label className="text-sm">Role</label>
                  <select className="mt-1 w-full border rounded-xl px-3 py-2" value={role} onChange={(e) => setRole(e.target.value as any)}>
                    <option value="student">student</option>
                    <option value="teacher">teacher</option>
                    <option value="admin">admin</option>
                  </select>
                </div>
                <button disabled={busy} className="bg-gray-900 text-white rounded-xl px-4 py-2">{busy ? "…" : "Login"}</button>
              </div>
            </form>
          )}

          {authed && !exam && (
            <div className="bg-white rounded-2xl shadow p-6 border max-w-lg">
              <h2 className="text-xl font-semibold">Load an exam</h2>
              <div className="mt-4 flex gap-2 items-end">
                <div className="flex-1">
                  <label className="text-sm">Exam ID</label>
                  <input className="mt-1 w-full border rounded-xl px-3 py-2" value={examId} onChange={(e) => setExamId(e.target.value)} />
                </div>
                <button onClick={fetchExam} className="bg-indigo-600 text-white rounded-xl px-4 py-2">Load</button>
              </div>
              <p className="text-xs text-gray-500 mt-3">Example: <code className="px-1.5 py-0.5 bg-gray-100 rounded">exam-101</code></p>
            </div>
          )}

          {exam && (
            <div className="bg-transparent">
              <div className="bg-white rounded-2xl shadow p-6 border">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <h2 className="text-xl font-semibold">{exam.title}</h2>
                    {exam.time_limit_sec ? (
                      <div className="text-xs text-gray-500">Time limit: {Math.round((exam.time_limit_sec || 0) / 60)} min</div>
                    ) : null}
                  </div>
                  {!attempt ? (
                    <button onClick={startAttempt} className="bg-emerald-600 text-white rounded-xl px-4 py-2">Start Attempt</button>
                  ) : (
                    <div className="text-sm">Attempt: <span className="font-mono">{attempt.id}</span> • {attempt.status}</div>
                  )}
                </div>

                {attempt && (
                  <div className="mt-6">
                    {/* Mobile question nav */}
                    <div className="lg:hidden mb-4">
                      <label className="text-sm">Jump to question</label>
                      <select
                        className="mt-1 border rounded-xl px-3 py-2 w-full"
                        value={currentQ}
                        onChange={(e) => setCurrentQ(Number(e.target.value))}
                      >
                        {exam.questions.map((q, idx) => (
                          <option key={q.id} value={idx}>Q{idx + 1}</option>
                        ))}
                      </select>
                    </div>

                    {/* Single-question view */}
                    {renderQuestion(exam.questions[currentQ])}

                    {/* Prev/Next */}
                    <div className="flex items-center gap-2">
                      <button
                        className="px-4 py-2 rounded-xl border hover:bg-gray-50"
                        onClick={() => setCurrentQ((i) => Math.max(0, i - 1))}
                        disabled={currentQ === 0}
                      >
                        ← Prev
                      </button>
                      <button
                        className="px-4 py-2 rounded-xl border hover:bg-gray-50"
                        onClick={() => setCurrentQ((i) => Math.min((exam.questions.length - 1), i + 1))}
                        disabled={currentQ >= exam.questions.length - 1}
                      >
                        Next →
                      </button>
                      <div className="ml-auto text-sm text-gray-500">Answered {answered} of {total}</div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}
        </section>
      </main>

      {/* Sticky action bar (only during attempt) */}
      {attempt && (
        <div className="sticky bottom-0 z-40 border-t bg-white/95 backdrop-blur">
          <div className="max-w-6xl mx-auto px-4 py-3 flex flex-wrap gap-2 items-center">
            <div className="text-xs text-gray-600">Autosaves as you type.</div>
            <button onClick={() => saveResponses(true)} className="ml-2 px-4 py-2 rounded-xl border hover:bg-gray-50">Save now</button>
            <label className={classNames("cursor-pointer px-4 py-2 rounded-xl border", uploading ? "opacity-60" : "hover:bg-gray-50 ml-1") }>
              <input type="file" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) uploadAsset(f); }} />
              {uploading ? "Uploading…" : "Upload scan"}
            </label>
            <button onClick={handleSubmitClick} className="ml-auto bg-blue-600 text-white rounded-xl px-4 py-2">Submit</button>
            {attempt?.score !== undefined && (
              <div className="ml-3 px-3 py-1 rounded-lg bg-emerald-50 text-emerald-800">Score: <b>{attempt.score}</b></div>
            )}
          </div>
        </div>
      )}

      {/* Toasts */}
      {(toast || error) && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2">
          <div className={classNames("rounded-xl shadow-xl px-4 py-2 text-sm", error ? "bg-red-600 text-white" : "bg-gray-900 text-white")}>
            {error || toast}
          </div>
        </div>
      )}

    {showConfirm && (
      <div className="fixed inset-0 flex items-center justify-center bg-black/50">
        <div className="bg-white rounded-xl p-6 shadow-lg">
          <p className="mb-4">Submit now? You won’t be able to edit after submitting.</p>
          <div className="flex gap-3">
            <button onClick={handleConfirmSubmit} className="bg-blue-600 text-white px-4 py-2 rounded-lg">Yes</button>
            <button onClick={() => setShowConfirm(false)} className="border px-4 py-2 rounded-lg">Cancel</button>
          </div>
        </div>
      </div>
    )}
    </div>
  );
}