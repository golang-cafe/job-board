# Developer Photo Validation and Privacy

Apply `migrations/108_developer_photo.sql` once to an existing database before
deploying. Fresh databases already include it in `latest-schema.sql`; do not
apply it twice. Existing profile image references are protected at read time,
without needing to revisit every stored photo. Orphaned uploads are only marked
private when uploaded as developer media or successfully validated for a profile.

Developer signup/edit sends `purpose=developer` to the existing image upload
endpoint. Detection runs on the actual crop before saving. Signup and profile
updates independently load and validate the submitted image ID, so uploading
through the company endpoint cannot bypass validation. Company uploads are not
subject to face detection. Old non-face profiles must upload a valid photo on
their next profile update.

The image is limited to 5MB upload / 4096 pixels per side / 16 million pixels
before decoding; detection operates on a maximum 640x640 image. A local Pigo
cascade requires a frontal face of at least 40 pixels and a confidence of 5.
This has false positives and false negatives. It is not identity, uniqueness,
humanity, or liveness verification, nor a complete anti-spam system. No external
image API, C binding, native detector binary, or runtime model download is used.

Unauthenticated and invalid/expired-session requests receive Gaussian-blurred,
re-encoded PNGs (maximum 512x512 before requested resizing). All developer image
responses, including authenticated originals, use `private, no-store` and
`Vary: Cookie`. Privacy transformation happens before size-query fallbacks.
Ordinary company media retains its existing caching behavior. Purge any previous
CDN/browser cache entries during deployment: headers cannot revoke old cached
originals. Face blur is a privacy mitigation, not guaranteed anonymization.

## Dependencies and Model Provenance

- Pure-Go detector: `github.com/esimov/pigo` v1.4.6, MIT.
- `internal/profilephoto/facefinder` is the unmodified model from
  https://github.com/esimov/pigo/blob/v1.4.6/cascade/facefinder .
  SHA256: `d8014993e7298c7b1865d1f8b855d6dbf4ec5c808bf879e2091ab6837abf90cd`.
  Its upstream license is preserved as `internal/profilephoto/PIGO-LICENSE`.
- Image resizing/blur: `github.com/disintegration/imaging` v1.6.2, MIT.

## Validation

`go test ./internal/profilephoto` runs blank/tiny image rejection checks.
Set `PHOTO_TEST_FACE` to a licensed PNG fixture to exercise positive detection.
No user photos should be used or uploaded in tests.

Set `PHOTO_TEST_DATABASE_URL` to an isolated PostgreSQL database to run
`go test -p=1 -vet=off ./... -count=1`. Tests use temporary image/profile tables,
apply the actual migration, and exercise upload, signup rejection, image-ID
revalidation, anonymous/logged-in/expired/forged-session reads, crop bounds,
resize fallbacks, company compatibility, and privacy cache headers. The existing
baseline fmt vet error in `handlers.go` is unrelated and is not hidden as a
passing default vet run. Database opt-in tests otherwise skip explicitly.

The development run used PostgreSQL WASM (PGlite) over loopback with Go lib/pq,
CGO disabled, and the public scikit-image NASA astronaut fixture. This validates
the SQL/HTTP paths, not production email/signup infrastructure or deployment.
Signup profile/token persistence uses fixture implementations and the application's
local-email mode; profile updates use the real SQL repository. No live email is sent.
The public fixture is available at
https://raw.githubusercontent.com/scikit-image/scikit-image/v0.19.3/skimage/data/astronaut.png .
Its public-domain status is documented at
https://scikit-image.org/docs/stable/api/skimage.data.html#skimage.data.astronaut .
It is deliberately not included in this repository or uploaded with test logs.
