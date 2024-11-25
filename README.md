# portfolio-yuanyuanzhou
## Workflow
1. Make changes to code.
2. Push code, merge master.
3. Build Docker image.
3. ssh to server, pull changes.
4. restart server, should pull new image.

## TODO
1. Redesign User table.
2. Activate login handler and page.
3. Make sure logging in and logging out works.

4. Redesign Posts table (probably want posts to be a set of one or more photos and videos.), So one-to-many from posts to objects.
5. Activate Upload page. The upload page should be able to upload arbitrary number of objects from phone and laptop, and add all of them to the objects table with same foreign key to Posts table.
6. Activate Post Page.
7.
