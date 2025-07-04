# portfolio-yuanyuanzhou
Performant and secure multiplexer to serve an artist portfolio. 

## Workflow
1. Make changes to code.
2. Push code, merge master.
3. Build Docker image.
4. ssh to server, pull changes.
5. restart server, should pull new image.

## TODO
- Improve this readme
- Add prettier styling (see bandcamp for example)
- Fix hovering on touch screen
- Fix the size of the cover image (on big screens it's just too big.)
- Serve large tumbnails rather than the original photo's.
- Let's rebuild the visual management api from the start:
  - Create visuak with just the filename in the database, but 
    - photos stored under /visuals/{visual-id}/filename
  - Patch it
