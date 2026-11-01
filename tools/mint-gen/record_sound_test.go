package main

import "testing"

const recorderHelp = `NAME
       micrec - record audio from a microphone

SYNOPSIS
       micrec [options] FILE

DESCRIPTION
       micrec records audio from the default capture device and writes it
       to FILE as WAV, at the sample rate and number of channels given.

OPTIONS
       -r, --rate RATE
              Sample rate in Hz.
       -c, --channels N
              Number of channels.
`

const audioConverterHelp = `NAME
       snd2x - convert audio files from one format to another

SYNOPSIS
       snd2x [options] INPUT OUTPUT

DESCRIPTION
       snd2x converts sound files between formats, and can change the
       sample rate and the number of channels on the way.

OPTIONS
       -f, --format FORMAT
              Output format.
`

func TestRecordSoundArchetype(t *testing.T) {
	p := programTextFrom("micrec", recorderHelp, "")
	p.Related = nil
	if c := chooseArchetypeFor(p); c.ID != "record-sound" {
		t.Fatalf("micrec chose %s", c.ID)
	}
	spec := ParseManAndHelp("micrec", recorderHelp, "")
	if spec.Data == "" {
		t.Fatal("a sound recording's data level must be sensitive")
	}
	// Talking about sound is not recording it.
	q := programTextFrom("snd2x", audioConverterHelp, "")
	q.Related = nil
	if c := chooseArchetypeFor(q); c.ID == "record-sound" {
		t.Fatal("an audio converter was taken for a recorder")
	}
}
