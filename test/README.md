# login as teacher
TOK=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"teacher","password":"teacher","role":"teacher"}' | jq -r '.access_token // .token')

# upload exam
curl -s -X POST http://localhost:8080/api/exams \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  --data @exam-101.json

# login as student
STOK=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"student","password":"student","role":"student"}' | jq -r '.access_token // .token')

# fetch exam
curl -s -H "Authorization: Bearer $STOK" \
  http://localhost:8080/api/exams/exam-101 | jq .

# create attempt
ATTEMPT=$(curl -s -X POST http://localhost:8080/api/attempts \
  -H "Authorization: Bearer $STOK" -H 'Content-Type: application/json' \
  -d '{"exam_id":"exam-101","user_id":"stu-1"}' | jq -r .id)

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
