"""Offline tests for tool_versions.py. Run: python3 -m unittest discover -s scripts -p 'test_*.py'"""

import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import tool_versions as tv  # noqa: E402

A, B, C, D = ("a" * 64), ("b" * 64), ("c" * 64), ("d" * 64)

MAKEFILE = f"""ATLAS_VERSION := v1.3.3
ATLAS_RELEASED := 2026-09-04
ATLAS_COMMUNITY_SHA256_linux-amd64  := {A}
ATLAS_COMMUNITY_SHA256_linux-arm64  := {B}
"""

DOCKERFILE = f"""ARG ATLAS_VERSION=v1.3.3
RUN case "$TARGETARCH" in \\
      amd64) sum={A} ;; \\
      arm64) sum={B} ;; \\
    esac
"""


class ToolVersionsTest(unittest.TestCase):
    def test_make_var_round_trip(self):
        text = tv.set_make_var(MAKEFILE, "ATLAS_VERSION", "v1.3.4")
        self.assertEqual(tv.make_var(text, "ATLAS_VERSION"), "v1.3.4")
        self.assertEqual(tv.make_var(text, "ATLAS_RELEASED"), "2026-09-04")

    def test_set_make_var_refuses_a_missing_line(self):
        with self.assertRaises(SystemExit):
            tv.set_make_var(MAKEFILE, "NOPE", "1")

    def test_checksum_lines_reads_both_formats(self):
        sums = tv.checksum_lines(f"{A}  golangci-lint-2.14.0-linux-amd64.tar.gz\n{B} *hadolint-linux-arm64\nnoise\n")
        self.assertEqual(sums, {"golangci-lint-2.14.0-linux-amd64.tar.gz": A, "hadolint-linux-arm64": B})

    def test_matching_pins_pass(self):
        self.assertEqual(tv.atlas_mismatches(MAKEFILE, DOCKERFILE), [])

    def test_version_mismatch_reported(self):
        problems = tv.atlas_mismatches(MAKEFILE, DOCKERFILE.replace("v1.3.3", "v1.3.2"))
        self.assertEqual(problems, ["ATLAS_VERSION: Makefile v1.3.3, Dockerfile v1.3.2"])

    def test_checksum_mismatch_reported(self):
        problems = tv.atlas_mismatches(MAKEFILE, DOCKERFILE.replace(f"arm64) sum={B}", f"arm64) sum={C}"))
        self.assertEqual(problems, ["Atlas community linux-arm64 checksum differs between Makefile and Dockerfile"])

    def test_rewrite_dockerfile_keeps_the_pair_in_step(self):
        text = tv.rewrite_dockerfile_atlas(DOCKERFILE, "1.3.4", C, D)
        makefile = MAKEFILE.replace("v1.3.3", "v1.3.4").replace(A, C).replace(B, D)
        self.assertEqual(tv.atlas_mismatches(makefile, text), [])


if __name__ == "__main__":
    unittest.main()
