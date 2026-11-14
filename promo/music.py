import numpy as np, sys, wave
dur=float(sys.argv[1]); out=sys.argv[2]; sr=44100
N=int(dur*sr); t=np.arange(N)/sr
L=np.zeros(N); R=np.zeros(N)
def hz(m): return 440*2**((m-69)/12)
bpm=84; beat=60/bpm; bar=4*beat
# Am9 - Fmaj7 - Cadd9 - G6 (midi)
chords=[[45,57,60,64,71],[41,57,60,64,69],[48,55,60,64,74],[43,55,59,62,64]]
rng=np.random.default_rng(3)
def env(n,a,r):
    e=np.ones(n); a=min(int(a*sr),n//2); r=min(int(r*sr),n//2)
    e[:a]=np.linspace(0,1,a); e[-r:]*=np.linspace(1,0,r); return e
nb=int(np.ceil(dur/bar))
for b in range(nb):
    ch=chords[(b//2)%4] if True else None
    s=int(b*bar*sr); n=min(int(bar*sr*1.25),N-s)
    if n<=0: break
    tt=np.arange(n)/sr; e=env(n,1.2,1.4)
    for i,m in enumerate(ch):
        f=hz(m+12 if i>0 else m)
        for d,pan in ((-0.07,0.3),(0.07,0.7)):
            ff=f*2**(d/12/1.0*0.1)
            w=np.sin(2*np.pi*ff*tt)+0.25*np.sin(2*np.pi*2*ff*tt)+0.08*np.sin(2*np.pi*3*ff*tt)
            g=(0.05 if i==0 else 0.028)*e
            L[s:s+n]+=w*g*(1-pan); R[s:s+n]+=w*g*pan
    # soft plucked arpeggio, 8ths
    if b>=1:
        notes=[ch[1]+12,ch[2]+12,ch[3]+12,ch[4]+12,ch[3]+12,ch[2]+12,ch[4]+12,ch[3]+12]
        for k,m in enumerate(notes):
            ps=s+int(k*beat/2*sr); pn=min(int(1.6*sr),N-ps)
            if pn<=0: continue
            pt=np.arange(pn)/sr; f=hz(m)
            w=(np.sin(2*np.pi*f*pt)+0.3*np.sin(2*np.pi*2*f*pt))*np.exp(-pt*3.2)*np.minimum(1,pt*200)
            pan=0.35+0.3*(k%2); g=0.045
            L[ps:ps+pn]+=w*g*(1-pan); R[ps:ps+pn]+=w*g*pan
    # sub pulse on 1
    ps=s; pn=min(int(beat*2*sr),N-ps); pt=np.arange(pn)/sr
    w=np.sin(2*np.pi*hz(ch[0]-12)*pt)*np.exp(-pt*1.5)*np.minimum(1,pt*60)*0.09
    L[ps:ps+pn]+=w; R[ps:ps+pn]+=w
# simple stereo delay "reverb"
for dl,fb in ((0.31,0.28),(0.47,0.22),(0.83,0.15)):
    d=int(dl*sr); L2=L.copy(); R2=R.copy()
    L2[d:]+=R[:-d]*fb; R2[d:]+=L[:-d]*fb; L,R=L2,R2
fi=int(1.5*sr); fo=int(3.0*sr)
for x in (L,R): x[:fi]*=np.linspace(0,1,fi); x[-fo:]*=np.linspace(1,0,fo)**1.5
m=max(np.abs(L).max(),np.abs(R).max()); L*=0.5/m; R*=0.5/m
d=(np.stack([L,R],1)*32767).astype('<i2')
with wave.open(out,'wb') as f: f.setnchannels(2); f.setsampwidth(2); f.setframerate(sr); f.writeframes(d.tobytes())
print('ok',out)
