# login as teacher
TOK=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"teacher","password":"teacher","role":"teacher"}' | jq -r '.access_token // .token')

# upload exam
curl -s -X POST http://localhost:8080/api/exams \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  --data @exam-001.json

# login as student
STOK=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"student","password":"student","role":"student"}' | jq -r '.access_token // .token')

# fetch exam
curl -s -H "Authorization: Bearer $STOK" \
  http://localhost:8080/api/exams/exam-001 | jq .

# create attempt
ATTEMPT=$(curl -s -X POST http://localhost:8080/api/attempts \
  -H "Authorization: Bearer $STOK" -H 'Content-Type: application/json' \
  -d '{"exam_id":"exam-001","user_id":"stu-1"}' | jq -r .id)

# save responses
curl -s -X POST http://localhost:8080/api/attempts/$ATTEMPT/responses \
  -H "Authorization: Bearer $STOK" -H 'Content-Type: application/json' \
  -d '{"q1":"9.8 m/s^2","q2":"true","q3":"v","q4":"3","q5":"My essay text..."}' | jq .

# submit
curl -s -X POST http://localhost:8080/api/attempts/$ATTEMPT/submit \
  -H "Authorization: Bearer $STOK" | jq .

# fetch the graded attempt as student
curl -s -H "Authorization: Bearer $STOK" \
  http://localhost:8080/api/attempts/$ATTEMPT | jq .

# (teacher view of same attempt)
curl -s -H "Authorization: Bearer $TOK" \
  http://localhost:8080/api/attempts/$ATTEMPT | jq .

# upload a scan (asset)
curl -s -X POST "http://localhost:8080/api/assets/$ATTEMPT" \
  -H "Authorization: Bearer $STOK" \
  -F "file=@math-scan.png" | jq .

# fetch it back
curl -s -H "Authorization: Bearer $STOK" \
  "http://localhost:8080/api/assets/attempts/$ATTEMPT/upload.bin" --output out.bin

# health check (root-level, not under /api)
curl -s http://localhost:8080/healthz


# create a course (teacher)
COURSE=$(curl -s -X POST http://localhost:8080/api/courses/ \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"name":"Physics 001 (link test)"}' | jq -r .id)
echo "COURSE=$COURSE"

# create a link offering for exam-101 with a DEMO123 token
# start_at = now-60, end_at = now+3600 (computed inline)
OFFER=$(curl -s -X POST http://localhost:8080/api/courses/$COURSE/offerings \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg exam "exam-001" --arg tok "DEMO123" --argjson now "$(date +%s)" \
        '{exam_id:$exam, start_at:($now-60), end_at:($now+3600),
          time_limit_sec:600, max_attempts:1, visibility:"link", access_token:$tok}')" \
  | jq -r .id)
echo "OFFER=$OFFER"

# public resolve (no JWT) using the link token
curl -s "http://localhost:8080/api/offerings/$OFFER/resolve?access_token=DEMO123" | jq .

# public ephemeral grading (no JWT). Replace qIDs to match your exam.
curl -s -X POST "http://localhost:8080/api/offerings/$OFFER/grade_ephemeral?access_token=DEMO123" \
  -H 'Content-Type: application/json' \
  -d '{"responses":{"q1":"9.8 m/s^2","q2":"true","q3":"v","q4":"3","q5":"My essay text..."}}' \
  | jq .

# optional: show correct answers too
curl -s -X POST "http://localhost:8080/api/offerings/$OFFER/grade_ephemeral?access_token=DEMO123&show_answers=1" \
  -H 'Content-Type: application/json' \
  -d '{"responses":{"q1":"9.8 m/s^2","q2":"true","q3":"v","q4":"3","q5":"My essay text..."}}' \
  | jq .

  curl -s "http://localhost:8080/api/offerings/$OFFER/ephemeral_stats?access_token=DEMO123" | jq .