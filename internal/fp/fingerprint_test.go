package fp

import "testing"

func TestNormalizeAndFingerprint(t *testing.T) {
    src := "  magnet:?xt=urn:btih:abc  "
    tgt := "  /tmp/dir/../file  "
    ns := NormalizeSource(src)
    if ns != "magnet:?xt=urn:btih:abc" {
        t.Fatalf("NormalizeSource: %q", ns)
    }
    nt := NormalizeTargetPath(tgt)
    if nt != "/tmp/file" {
        t.Fatalf("NormalizeTargetPath: %q", nt)
    }

    fp1 := Fingerprint(src, tgt)
    fp2 := Fingerprint("magnet:?xt=urn:btih:abc", "/tmp/file")
    if fp1 != fp2 {
        t.Fatalf("fingerprints differ: %s vs %s", fp1, fp2)
    }
    if len(fp1) != 64 { // hex-encoded sha256
        t.Fatalf("unexpected fp length: %d", len(fp1))
    }
}

