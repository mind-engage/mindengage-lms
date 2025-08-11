```
# login as teacher
TOK=$(curl -s -X POST localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"teacher","password":"teacher","role":"teacher"}' | jq -r .access_token)

# upload exam
curl -s -X POST localhost:8080/exams \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  --data @exam-101.json

# login as student
STOK=$(curl -s -X POST localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"student","password":"student","role":"student"}' | jq -r .access_token)

# fetch exam
curl -s -H "Authorization: Bearer $STOK" localhost:8080/exams/exam-101 | jq .

# create attempt
ATTEMPT=$(curl -s -X POST localhost:8080/attempts \
  -H "Authorization: Bearer $STOK" -H 'Content-Type: application/json' \
  -d '{"exam_id":"exam-101","user_id":"stu-1"}' | jq -r .id)

# save responses
curl -s -X POST localhost:8080/attempts/$ATTEMPT/responses \
  -H "Authorization: Bearer $STOK" -H 'Content-Type: application/json' \
  -d '{"q1":"9.8 m/s^2","q2":"true","q3":"v","q4":"3","q5":"My essay text..."}' | jq .

# submit
curl -s -X POST localhost:8080/attempts/$ATTEMPT/submit \
  -H "Authorization: Bearer $STOK" | jq .

# fetch the graded attempt as student
curl -s -H "Authorization: Bearer $STOK" \
  localhost:8080/attempts/$ATTEMPT | jq .

This should now include "score" and "status": "submitted".

curl -s -H "Authorization: Bearer $TOK" \
  localhost:8080/attempts/$ATTEMPT | jq .

If your API is role-aware, the teacher might see answer keys and more grading info.

# upload a scan
curl -s -X POST "localhost:8080/assets/$ATTEMPT" \
  -H "Authorization: Bearer $STOK" \
  -F "file=@math-scan.png" | jq .

# fetch it back
curl -s -H "Authorization: Bearer $STOK" \
  "localhost:8080/assets/attempts/$ATTEMPT/upload.bin" --output out.bin

# health check
curl -s -H "Authorization: Bearer $STOK" \
  localhost:8080/healthz | jq .

```


Upload formats (front-end friendly)
CSV (recommended for teachers)

Header row required:

id,username,role,password
s-001,alice,student,alicepass
s-002,bob,student,bobpass

JSON

[
  {"id":"s-001","username":"alice","role":"student","password":"alicepass"},
  {"id":"s-002","username":"bob","role":"student","password":"bobpass"}
]

cURL examples

# Bulk upload CSV
curl -s -X POST http://localhost:8080/users/bulk \
  -H "Authorization: Bearer $TOK" \
  -F "file=@students.csv" | jq .

# Bulk upload JSON (raw body)
curl -s -X POST http://localhost:8080/users/bulk \
  -H "Authorization: Bearer $TOK" \
  -H "Content-Type: application/json" \
  --data-binary @students.json | jq .

# List students
curl -s -H "Authorization: Bearer $TOK" \
  'http://localhost:8080/users?role=student' | jq .


# Change password as a student
curl -X POST http://localhost:8080/users/change-password \
  -H "Authorization: Bearer $STOK" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"oldpass123","new_password":"newpass456"}'

# import qti zip
curl -s -X POST "http://localhost:8080/qti/import" \
  -H "Authorization: Bearer $TOK" \
  -F "file=@test/sample-qti.zip" | jq .

# export qti zip
curl -s -H "Authorization: Bearer $TOK" \
  "http://localhost:8080/exams/exam-101/export?format=qti" \
  --output exported.zip  