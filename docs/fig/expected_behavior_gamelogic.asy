unitsize(1cm);

real margin=1mm;
real y = 0;
real yoff = -2;
real xoff = 4;

object start = draw("", ellipse, (0,y), 0); y += yoff;
object unlogged = draw("unlogged", ellipse, (0,y), margin); y += yoff;
object waiting = draw("waiting logging answer", ellipse, (0,y), margin); y += yoff;
object logged = draw("logged", ellipse, (0,y), margin); y += yoff;
object gameinit = draw("initializing", ellipse, (0,y), margin); y += yoff;
object listening = draw("listening", ellipse, (-xoff,y), margin);
object thinking = draw("computing turn", ellipse, (xoff,y), margin); y += yoff;
object gameover = draw("can close socket", ellipse, (0,y), margin); y += yoff;

add(new void(picture pic, transform t)
{
    draw(pic, "connect socket", point(start,S,t)..point(unlogged,N,t),
         fontsize(10), Arrow);
});

add(new void(picture pic, transform t)
{
    draw(pic, "!LOGIN", point(unlogged,S,t)..point(waiting,N,t),
         fontsize(10), Arrow);
});

add(new void(picture pic, transform t)
{
    draw(pic, "?LOGIN\_ACK", point(waiting,S,t)..point(logged,N,t),
         fontsize(10), Arrow);
});

add(new void(picture pic, transform t)
{
    draw(pic, "?DO\_INIT(nb\_turns\_max=X)", point(logged,S,t)..point(gameinit,N,t),
         fontsize(10), Arrow);
});

add(new void(picture pic, transform t)
{
    draw(pic, "!DO\_INIT\_ACK", point(gameinit,S,t)..point(listening,N,t),
         fontsize(10), Arrow);
});

add(new void(picture pic, transform t)
{
    draw(pic, "?DO\_TURN", point(listening,N,t)..point(thinking,N,t),
         fontsize(10), Arrow);
});

add(new void(picture pic, transform t)
{
    draw(pic, "!DO\_TURN\_ACK", point(thinking,S,t)..point(listening,S,t),
         fontsize(10), Arrow);
});

add(new void(picture pic, transform t)
{
    draw(pic, "X DO\_TURN\_ACK have been sent",
         point(listening,S,t)..point(gameover,N,t),
         fontsize(10), Arrow);
});
